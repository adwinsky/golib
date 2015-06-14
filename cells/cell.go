// Tideland Go Library - Cells - Cell
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
	"time"

	"github.com/tideland/golib/errors"
	"github.com/tideland/golib/identifier"
	"github.com/tideland/golib/logger"
	"github.com/tideland/golib/loop"
	"github.com/tideland/golib/monitoring"
	"github.com/tideland/golib/scene"
)

//--------------------
// CELL
//--------------------

// cell for event processing.
type cell struct {
	env         *environment
	id          string
	behavior    Behavior
	subscribers []*cell
	queue       EventQueue
	subscriberc chan []*cell
	loop        loop.Loop
	measuringID string
}

// newCell create a new cell around a behavior.
func newCell(env *environment, id string, behavior Behavior) (*cell, error) {
	// Create queue.
	queue, err := env.queueFactory(env)
	if err != nil {
		return nil, errors.Annotate(err, ErrCellInit, errorMessages, id)
	}
	// Init cell runtime.
	c := &cell{
		env:         env,
		id:          id,
		behavior:    behavior,
		queue:       queue,
		subscriberc: make(chan []*cell),
		measuringID: identifier.Identifier("cells", env.id, "cell", identifier.TypeAsIdentifierPart(behavior)),
	}
	c.loop = loop.GoRecoverable(c.backendLoop, c.checkRecovering)
	// Init behavior.
	if err := behavior.Init(c); err != nil {
		return nil, errors.Annotate(err, ErrCellInit, errorMessages, id)
	}
	logger.Infof("cell %q started", id)
	return c, nil
}

// Environment implements the Context interface.
func (c *cell) Environment() Environment {
	return c.env
}

// ID implements the Context interface.
func (c *cell) ID() string {
	return c.id
}

// Emit implements the Context interface.
func (c *cell) Emit(event Event) error {
	for _, ec := range c.subscribers {
		if err := ec.processEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// EmitNew implements the Context interface.
func (c *cell) EmitNew(topic string, payload interface{}, scene scene.Scene) error {
	event, err := NewEvent(topic, payload, scene)
	if err != nil {
		return err
	}
	return c.Emit(event)
}

// updateSubscribers sets the subscribers of the cell.
func (c *cell) updateSubscribers(cells []*cell) {
	c.subscriberc <- cells
}

// processEvent tells the cell to process an event.
func (c *cell) processEvent(event Event) error {
	return c.queue.Push(event)
}

// stop terminates the cell.
func (c *cell) stop() error {
	return c.loop.Stop()
}

// backendLoop is the backend for the processing of messages.
func (c *cell) backendLoop(l loop.Loop) error {
	monitoring.IncrVariable(c.measuringID)
	monitoring.IncrVariable(identifier.Identifier("cells", c.env.ID(), "total-cells"))
	defer monitoring.DecrVariable(identifier.Identifier("cells", c.env.ID(), "total-cells"))
	defer monitoring.DecrVariable(c.measuringID)
	defer c.cleanup()

	for {
		select {
		case <-c.loop.ShallStop():
			return c.behavior.Terminate()
		case subscribers := <-c.subscriberc:
			c.subscribers = subscribers
		case event := <-c.queue.Events():
			// if event == nil {
			//	panic("received illegal nil event!")
			// }
			measuring := monitoring.BeginMeasuring(c.measuringID)
			err := c.behavior.ProcessEvent(event)
			if err != nil {
				c.loop.Kill(err)
				continue
			}
			measuring.EndMeasuring()
		}
	}
}

// checkRecovering checks if the cell may recover after a panic. It will
// signal an error and let the cell stop working if there have been 12 recoverings
// during the last minute or the behaviors Recover() signals, that it cannot
// handle the error.
func (c *cell) checkRecovering(rs loop.Recoverings) (loop.Recoverings, error) {
	logger.Errorf("recovering cell %q after error: %v", c.id, rs.Last().Reason)
	// Check frequency.
	if rs.Frequency(12, time.Minute) {
		return nil, errors.New(ErrRecoveredTooOften, errorMessages, rs.Last().Reason)
	}
	// Try to recover.
	if err := c.behavior.Recover(rs.Last().Reason); err != nil {
		return nil, errors.Annotate(err, ErrEventRecovering, errorMessages, rs.Last().Reason)
	}
	return rs.Trim(12), nil
}

// cleanup ensures a proper end of a cell.
func (c *cell) cleanup() {
	// Unsubscribe from subscriptions.
	// Notify subscribers.
	// Terminate the queue.
	if err := c.queue.Stop(); err != nil {
		logger.Errorf("cannot stop queue of cell %q: %v", c.id, err)
	}
	logger.Infof("cell %q terminated", c.id)
}

// EOF
