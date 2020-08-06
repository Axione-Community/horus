package agent

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kosctelecom/horus/log"
	"github.com/nats-io/nats.go"
)

// NatsClient is a NATS publisher sink for snmp results
type NatsClient struct {
	// Hosts is the list of NATS urls
	Hosts []string

	// Topic is the NATS topic (subject) to use
	Topic string

	// Name is the NATS connection name
	Name string

	nc *nats.Conn
	ec *nats.EncodedConn
}

var natsCli *NatsClient

// NewNatsClient creates a new NATS client and connects to server.
func NewNatsClient(hosts []string, topic, name string) error {
	if len(hosts) == 0 || topic == "" {
		return fmt.Errorf("NATS host and topic must all be defined")
	}
	if name == "" {
		name = fmt.Sprintf("horus-agent[%d]", os.Getpid())
	}
	natsCli = &NatsClient{
		Hosts: hosts,
		Topic: topic,
		Name:  name,
	}
	log.Debug2f("connecting to NATS %v", hosts)
	nc, err := nats.Connect(strings.Join(hosts, ","), nats.Name(name))
	if err != nil {
		return fmt.Errorf("NATS dial: %v", err)
	}
	natsCli.nc = nc
	log.Debugf("connected to NATS")
	ec, err := nats.NewEncodedConn(nc, nats.JSON_ENCODER)
	if err != nil {
		return fmt.Errorf("NATS encoded conn: %v", err)
	}
	natsCli.ec = ec
	return nil
}

// Close closes the NATS connection.
func (c *NatsClient) Close() {
	c.ec.Close()
	c.nc.Close()
}

// Push publishes the poll result to NATS
func (c *NatsClient) Push(res PollResult) {
	if c == nil {
		return
	}
	res = res.Copy()
	start := time.Now()
	if err := c.ec.Publish(c.Topic, res); err != nil {
		log.Errorf("%s: NATS publish: %v", res.RequestID, err)
	}
	if err := c.nc.FlushTimeout(time.Second); err != nil {
		log.Errorf("NATS queue flush: %v", err)
		return
	}
	log.Debug2f("NATS publish req %s done in %dms", res.RequestID, time.Since(start)/time.Millisecond)
}
