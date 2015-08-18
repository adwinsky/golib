// Tideland Go Library - Cells - Environment
//
// Copyright (C) 2010-2015 Frank Mueller / Tideland / Oldenburg / Germany
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

package cells

//--------------------
// IMPORTS
//--------------------

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/tideland/golib/errors"
	"github.com/tideland/golib/identifier"
	"github.com/tideland/golib/logger"
	"github.com/tideland/golib/scene"
)

//--------------------
// ENVIRONMENT
//--------------------

// Environment implements the Environment interface.
type environment struct {
	mutex   sync.RWMutex
	id      string
	cells   *registry
	monitor Monitoring
}

// NewEnvironment creates a new environment.
func NewEnvironment(idParts ...interface{}) Environment {
	var id string
	if len(idParts) == 0 {
		id = identifier.NewUUID().String()
	} else {
		id = identifier.Identifier(idParts...)
	}
	env := &environment{
		id:      id,
		cells:   newRegistry(),
		monitor: NewNullMonitoring(),
	}
	runtime.SetFinalizer(env, (*environment).Stop)
	logger.Infof("cells environment %q started", env.ID())
	return env
}

// ID implements the Environment interface.
func (env *environment) ID() string {
	env.mutex.RLock()
	defer env.mutex.RUnlock()

	return env.id
}

// SetMonitoring implements the Environment interface.
func (env *environment) SetMonitoring(m Monitoring) {
	env.mutex.Lock()
	defer env.mutex.Unlock()

	env.monitor = m
}

// monitoring provides access to the environments monitoring.
func (env *environment) monitoring() Monitoring {
	env.mutex.RLock()
	defer env.mutex.RUnlock()

	return env.monitor
}

// StartCell implements the Environment interface.
func (env *environment) StartCell(id string, behavior Behavior) error {
	return env.cells.startCell(env, id, behavior)
}

// StopCell implements the Environment interface.
func (env *environment) StopCell(id string) error {
	return env.cells.stopCell(id)
}

// HasCell implements the Environment interface.
func (env *environment) HasCell(id string) bool {
	_, err := env.cells.cells(id)
	return err == nil
}

// Subscribe implements the Environment interface.
func (env *environment) Subscribe(emitterID string, subscriberIDs ...string) error {
	return env.cells.subscribe(emitterID, subscriberIDs...)
}

// Subscribers implements the Environment interface.
func (env *environment) Subscribers(id string) ([]string, error) {
	return env.cells.subscribers(id)
}

// Unsubscribe implements the Environment interface.
func (env *environment) Unsubscribe(emitterID string, subscriberIDs ...string) error {
	return env.cells.unsubscribe(emitterID, subscriberIDs...)
}

// Emit implements the Environment interface.
func (env *environment) Emit(id string, event Event) error {
	cs, err := env.cells.cells(id)
	if err != nil {
		return err
	}
	return cs[0].ProcessEvent(event)
}

// EmitNew implements the Environment interface.
func (env *environment) EmitNew(id, topic string, payload interface{}, scene scene.Scene) error {
	event, err := NewEvent(topic, payload, scene)
	if err != nil {
		return err
	}
	return env.Emit(id, event)
}

// Request implements the Environment interface.
func (env *environment) Request(
	id, topic string,
	payload interface{},
	scn scene.Scene,
	timeout time.Duration,
) (interface{}, error) {
	responseChan := make(chan interface{}, 1)
	p := NewPayload(payload).Apply(PayloadValues{ResponseChanPayload: responseChan})
	err := env.EmitNew(id, topic, p, scn)
	if err != nil {
		return nil, err
	}
	select {
	case response := <-responseChan:
		if err, ok := response.(error); ok {
			return nil, err
		}
		return response, nil
	case <-time.After(timeout):
		op := fmt.Sprintf("requesting %q from %q", topic, id)
		return nil, errors.New(ErrTimeout, errorMessages, op)
	}
}

// Stop implements the Environment interface.
func (env *environment) Stop() error {
	runtime.SetFinalizer(env, nil)
	if err := env.cells.stop(); err != nil {
		return err
	}
	logger.Infof("cells environment %q terminated", env.ID())
	return nil
}

// EOF
