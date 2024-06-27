package collection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/websocket"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

// client is a middleman between the websocket connection and the hub.
type client struct {
	hub    *hub
	logger log.Logger

	// The websocket connection.
	conn *websocket.Conn

	role             Role
	subscribedTopics []string

	// Buffered channel of outbound messages.
	send      chan []byte
	sendClose sync.Once
}

func (c *client) close() {
	c.sendClose.Do(func() {
		close(c.send)
	})
}

func (c *client) isRuleManager() bool {
	return c.role&RuleManager == RuleManager
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *client) readPump() {
	defer func() {
		c.hub.unregisterCh <- c
		c.close()
		c.conn.Close()
	}()
	var (
		msg settingsv1.CollectionMessage
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		msg.Reset()
		if err := c.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				level.Warn(c.logger).Log("msg", "websocket abnormal", "error", err)
			} else if errors.Is(err, net.ErrClosed) {
				level.Debug(c.logger).Log("msg", "websocket underlying connection closed", "error", err)
			} else {
				level.Error(c.logger).Log("msg", "error reading json", "error", err)
			}
			break
		}

		handleResult := func(messageID int64, err error) {
			if messageID == 0 {
				return
			}
			msg := &settingsv1.CollectionMessage{
				Id:     messageID,
				Status: settingsv1.Status_STATUS_OK,
			}
			if err != nil {
				level.Error(c.logger).Log("error", err)
				msg.Status = settingsv1.Status_STATUS_ERROR
				errDetail := err.Error()
				msg.Message = &errDetail
			}
			d, _ := json.Marshal(msg)
			c.send <- d
		}

		if p := msg.PayloadSubscribe; p != nil {
			c.subscribedTopics = p.Topics
			level.Debug(c.logger).Log("msg", "client subscribing", "topics", fmt.Sprintf("%v", p.Topics))
			c.hub.registerCh <- c
		} else if p := msg.PayloadData; p != nil {

			for idx := range p.Instances {
				i := p.Instances[idx]
				level.Debug(c.logger).Log("msg", "received collection instance targets", "hostname", i.Hostname, "targets", len(i.Targets))
				c.hub.instanceCh <- i
			}
		} else if p := msg.PayloadRuleDelete; p != nil {
			if p.Id <= 0 {
				level.Warn(c.logger).Log("msg", "received rule delete without id")
				continue
			}
			if !c.isRuleManager() {
				level.Warn(c.logger).Log("msg", "not allowed for collection instance")
				continue
			}
			level.Info(c.logger).Log("msg", "received rule delete", "id", p.Id)
			id := p.Id
			c.hub.rulesCh <- func(h *hub) {
				handleResult(msg.Id, h.deleteRule(ctx, id))
			}
		} else if p := msg.PayloadRuleInsert; p != nil {
			if !c.isRuleManager() {
				level.Warn(c.logger).Log("msg", "not allowed without rule manager role")
				continue
			}
			level.Info(c.logger).Log("msg", "received rule insert", "rule", p.Rule)
			c.hub.rulesCh <- func(h *hub) {
				handleResult(msg.Id, h.insertRule(ctx, p))
			}
		} else {
			level.Warn(c.logger).Log("msg", "no known message type used")
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for message := range c.send {

		w, err := c.conn.NextWriter(websocket.TextMessage)
		if err != nil {
			level.Warn(c.logger).Log("msg", "failed creating writer for client", "error", err)
			return
		}
		messageLen, err := w.Write(message)
		if len(message) > 64 {
			message = append(message[:64], []byte("...")...)
		}
		if err != nil {
			level.Warn(c.logger).Log("msg", "failed writing message to client", "error", err)
			return
		}

		if err := w.Close(); err != nil {
			level.Warn(c.logger).Log("msg", "failed closing message to client", "error", err)
			return
		}
		level.Debug(c.logger).Log("msg", "sent message to client", "size", messageLen, "message", message)
	}

	// The hub closed the channel.
	_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
}
