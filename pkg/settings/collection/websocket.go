package collection

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/websocket"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

var (
	newline = []byte{'\n'}
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub    *Hub
	logger log.Logger

	// The websocket connection.
	conn *websocket.Conn

	role Role

	subscribedTopics []string

	// Buffered channel of outbound messages.
	send chan []byte
}

type recvMsg = settingsv1.CollectionMessage

/*
type recvMsg struct {
	Type       model.MessageType `json:"type"`
	Data       *model.PayloadData
	Subscribe  *model.PayloadSubscribe
	RuleDelete *model.PayloadRuleDelete
	RuleInsert *model.PayloadRuleInsert
}

func (m *recvMsg) UnmarshalJSON(b []byte) error {
	var header struct {
		Type    model.MessageType `json:"type"`
		Payload json.RawMessage   `json:"payload"`
	}
	json.Unmarshal(b, &header)
	m.Type = header.Type
	m.Data = nil
	m.Subscribe = nil

	switch m.Type {
	case model.MessageTypeData:
		var data model.PayloadData
		if err := json.Unmarshal(header.Payload, &data); err != nil {
			return err
		}
		m.Data = &data
	case model.MessageTypeSubscribe:
		var subscribe model.PayloadSubscribe
		if err := json.Unmarshal(header.Payload, &subscribe); err != nil {
			return err
		}
		m.Subscribe = &subscribe
	case model.MessageTypeRuleInsert:
		var e model.PayloadRuleInsert
		if err := json.Unmarshal(header.Payload, &e); err != nil {
			return err
		}
		m.RuleInsert = &e
	case model.MessageTypeRuleDelete:
		var e model.PayloadRuleDelete
		if err := json.Unmarshal(header.Payload, &e); err != nil {
			return err
		}
		m.RuleDelete = &e
	default:
		return fmt.Errorf("unknown message type %s", m.Type)
	}
	return nil
}
*/

func (c *Client) isRuleManager() bool {
	return c.role&RuleManager == RuleManager
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregisterCh <- c
		c.conn.Close()
	}()
	var (
		msg settingsv1.CollectionMessage
	)

	for {
		if err := c.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				level.Error(c.logger).Log("msg", "error reading json", "error", err)
			}
			break
		}

		switch p := msg.Payload.(type) {
		case *settingsv1.CollectionMessage_PayloadSubscribe:

			c.subscribedTopics = p.PayloadSubscribe.Topics
			level.Debug(c.logger).Log("msg", "client subscribing", "topics", p.PayloadSubscribe.Topics)
			c.hub.registerCh <- c
		case *settingsv1.CollectionMessage_PayloadData:
			for idx := range p.PayloadData.Instances {
				a := p.PayloadData.Instances[idx]
				level.Debug(c.logger).Log("msg", "received collection instance targets", "hostname", a.Hostname, "targets", len(a.Targets))
				c.hub.agentCh <- a
			}
		case *settingsv1.CollectionMessage_PayloadRuleDelete:
			if p.PayloadRuleDelete.Id <= 0 {
				level.Warn(c.logger).Log("msg", "received rule delete without id")
				continue
			}
			if !c.isRuleManager() {
				level.Warn(c.logger).Log("msg", "not allowed for collection instance")
				continue
			}
			level.Debug(c.logger).Log("msg", "received rule delete", "id", p.PayloadRuleDelete.Id)
			id := p.PayloadRuleDelete.Id
			c.hub.rulesCh <- func(h *Hub) {
				h.deleteRule(id)
			}
		case *settingsv1.CollectionMessage_PayloadRuleInsert:
			if !c.isRuleManager() {
				level.Warn(c.logger).Log("msg", "not allowed without rule manager role")
				continue
			}
			level.Debug(c.logger).Log("msg", "received rule insert", "rule", p.PayloadRuleInsert.Rule)
			c.hub.rulesCh <- func(h *Hub) {
				h.insertRule(p.PayloadRuleInsert)
			}

		default:
			level.Warn(c.logger).Log("msg", "unknown message type", "type", msg.Payload)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			messageLen, _ := w.Write(message)
			if len(message) > 64 {
				message = append(message[:64], []byte("...")...)
			}
			level.Debug(c.logger).Log("msg", "sent message to client", "size", messageLen, "message", message)

			if err := w.Close(); err != nil {
				return
			}
		}
	}
}
