/*
 * Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package systemd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

// systemdConnectTimeout bounds how long NewManager waits for the systemd D-Bus
// connection (dial plus auth handshake) to be established. On hosts without a
// running systemd/D-Bus daemon the system bus socket may still exist - for
// example when /run/dbus/system_bus_socket is bind-mounted from the host - but
// never answer, in which case the underlying auth handshake would otherwise
// block forever. Bounding it turns that indefinite hang into a fast, actionable
// error.
const systemdConnectTimeout = 10 * time.Second

// Manager handles systemd operations using the D-Bus API
type Manager struct {
	ctx context.Context

	conn   *dbus.Conn
	cancel context.CancelFunc
}

// NewManager creates a new Manager instance connected to the systemd system
// bus. It fails fast, rather than blocking indefinitely, when the D-Bus socket
// exists but no systemd daemon is answering (as happens on systemd-less hosts
// such as Talos Linux).
func NewManager(ctx context.Context) (*Manager, error) {
	return newManagerWithTimeout(ctx, systemdConnectTimeout)
}

// newManagerWithTimeout is the testable core of NewManager, with the connection
// timeout injected so tests do not have to wait for the production default.
func newManagerWithTimeout(ctx context.Context, timeout time.Duration) (*Manager, error) {
	// Derive a cancelable context for the connection. go-systemd/godbus tie the
	// lifetime of the connection to this context and, crucially, spawn a watcher
	// goroutine that closes the underlying socket as soon as the context is done.
	// We rely on that to unblock a stuck auth handshake: if the dial does not
	// complete within the timeout we cancel the context, the socket is closed,
	// and the blocked read returns an error instead of hanging forever.
	connCtx, cancel := context.WithCancel(ctx)

	type result struct {
		conn *dbus.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := dbus.NewSystemConnectionContext(connCtx)
		ch <- result{conn: conn, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case res := <-ch:
		if res.err != nil {
			cancel()
			return nil, fmt.Errorf("failed to connect to systemd D-Bus: %w", res.err)
		}
		// Success: the connection (and connCtx) must outlive this call, so we do
		// not cancel here. cancel is retained and invoked by Close().
		return &Manager{
			ctx:    ctx,
			conn:   res.conn,
			cancel: cancel,
		}, nil
	case <-timer.C:
		// The dial is stuck: the socket exists but nothing is answering, as on a
		// systemd-less host. Cancel the connection context so the godbus watcher
		// closes the socket and unblocks the goroutine above, then report a clear
		// error instead of hanging.
		cancel()
		return nil, fmt.Errorf("timed out after %s connecting to systemd D-Bus: the system bus socket exists but is not responding (is this a systemd-less host?)", timeout)
	}
}

// Close closes the D-Bus connection
func (sm *Manager) Close() error {
	if sm.conn != nil {
		sm.conn.Close()
	}
	if sm.cancel != nil {
		sm.cancel()
	}
	return nil
}

// ServiceStatus represents the status of a systemd service
type ServiceStatus struct {
	Name     string
	Active   bool
	Enabled  bool
	Failed   bool
	Type     string
	SubState string
}

// GetServiceStatus gets the status of a systemd service
func (sm *Manager) GetServiceStatus(serviceName string) (*ServiceStatus, error) {

	unitStatus, err := sm.getUnitStatus(serviceName)
	if err != nil {
		return nil, err
	}

	if unitStatus.LoadState == "not-found" {
		return &ServiceStatus{
			Name:     serviceName,
			Active:   false,
			Enabled:  false,
			Failed:   false,
			Type:     "",
			SubState: "not-found",
		}, nil
	}

	// Get unit properties
	properties, err := sm.conn.GetAllPropertiesContext(sm.ctx, serviceName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return &ServiceStatus{
				Name:     serviceName,
				Active:   false,
				Enabled:  false,
				Failed:   false,
				Type:     "",
				SubState: "not-found",
			}, nil
		}
		return nil, fmt.Errorf("failed to get properties for service %s: %w", serviceName, err)
	}

	status := &ServiceStatus{
		Name: serviceName,
	}

	if activeState, ok := properties["ActiveState"].(string); ok {
		status.Active = activeState == "active"
	}

	if subState, ok := properties["SubState"].(string); ok {
		status.SubState = subState
	}

	if unitFileState, ok := properties["UnitFileState"].(string); ok {
		status.Enabled = unitFileState == "enabled"
	}

	if loadState, ok := properties["LoadState"].(string); ok {
		status.Failed = loadState == "not-found" || loadState == "error"
	}

	if unitType, ok := properties["Type"].(string); ok {
		status.Type = unitType
	}

	return status, nil
}

// StartService starts a systemd service
func (sm *Manager) StartService(serviceName string) error {
	ch := make(chan string, 1)
	_, err := sm.conn.StartUnitContext(sm.ctx, serviceName, "replace", ch)
	if err != nil {
		return fmt.Errorf("failed to start service %s: %w", serviceName, err)
	}

	// Wait for the operation to complete
	select {
	case result := <-ch:
		if result != "done" {
			return fmt.Errorf("failed to start service %s: %s", serviceName, result)
		}
	case <-time.After(120 * time.Second):
		return fmt.Errorf("timeout starting service %s", serviceName)
	}

	return nil
}

// StopService stops a systemd service
func (sm *Manager) StopService(serviceName string) error {
	ch := make(chan string, 1)
	_, err := sm.conn.StopUnitContext(sm.ctx, serviceName, "replace", ch)
	if err != nil {
		return fmt.Errorf("failed to stop service %s: %w", serviceName, err)
	}

	// Wait for the operation to complete
	select {
	case result := <-ch:
		if result != "done" {
			return fmt.Errorf("failed to stop service %s: %s", serviceName, result)
		}
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout stopping service %s", serviceName)
	}

	return nil
}

// ReloadDaemon reloads the systemd daemon configuration
func (sm *Manager) ReloadDaemon() error {
	err := sm.conn.ReloadContext(sm.ctx)
	if err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %w", err)
	}
	return nil
}

// StopSystemdServices stops multiple systemd services with proper status checking
func (sm *Manager) StopSystemdServices(services []string) ([]string, error) {
	var stoppedServices []string

	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}

		status, err := sm.GetServiceStatus(service)
		if err != nil {
			fmt.Printf("Skipping %s (error checking status: %v)\n", service, err)
			continue
		}

		// If the service is "active" we will attempt to shut it down and (if
		// successful) we will track it to restart it later.
		if status.Active {
			fmt.Printf("Stopping %s (active, will-restart)\n", service)
			if err := sm.StopService(service); err != nil {
				return stoppedServices, fmt.Errorf("failed to stop service %s: %w", service, err)
			}
			stoppedServices = append(stoppedServices, service)
			continue
		}

		// If the service is inactive, then we may or may not still want to track
		// it to restart it later. The logic below decides when we should or not.
		if status.SubState == "not-found" {
			fmt.Printf("Skipping %s (no-exist)\n", service)
			continue
		}

		if status.Failed {
			fmt.Printf("Skipping %s (is-failed, will-restart)\n", service)
			stoppedServices = append(stoppedServices, service)
			continue
		}

		if !status.Enabled {
			fmt.Printf("Skipping %s (disabled)\n", service)
			continue
		}

		if status.Type == "oneshot" {
			fmt.Printf("Skipping %s (inactive, oneshot, no-restart)\n", service)
			continue
		}

		fmt.Printf("Skipping %s (inactive, will-restart)\n", service)
		stoppedServices = append(stoppedServices, service)
	}

	// We reverse the slice of stoppedServices as we want the LIFO execution order when restarting them
	slices.Reverse(stoppedServices)

	return stoppedServices, nil
}

// StartSystemdServices starts multiple systemd services
func (sm *Manager) StartSystemdServices(services []string) error {
	var ret error

	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}

		fmt.Printf("Starting %s\n", service)
		if err := sm.StartService(service); err != nil {
			fmt.Printf("Error Starting %s: skipping, but continuing...\n", service)
			ret = errors.Join(ret, err)
		}
	}

	return ret
}

func (sm *Manager) getUnitStatus(service string) (*dbus.UnitStatus, error) {
	unitStatuses, err := sm.conn.ListUnitsByNamesContext(sm.ctx, []string{service})
	if err != nil {
		return nil, fmt.Errorf("failed to get unit status for %s: %w", service, err)
	}

	return &unitStatuses[0], nil
}
