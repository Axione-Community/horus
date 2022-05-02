package agent

import (
	"context"
	"fmt"

	"github.com/gosnmp/gosnmp"
)

// GoSNMPWrapper adds context support to gosnmp
type GoSNMPWrapper struct {
	*gosnmp.GoSNMP
}

type snmpResult struct {
	pkt *gosnmp.SnmpPacket
	err error
}

// DialWithCtx connects through udp with a context
func (w *GoSNMPWrapper) DialWithCtx(ctx context.Context) error {
	w.Context = ctx
	errCh := make(chan error)
	go func() {
		errCh <- w.Connect()
	}()
	select {
	case <-ctx.Done():
		if w.Conn != nil {
			w.Conn.Close()
		}
		return fmt.Errorf("request cancelled while connecting")
	case err := <-errCh:
		return err
	}
}

// GetWithCtx calls Get with a context
func (w *GoSNMPWrapper) GetWithCtx(ctx context.Context, oids []string) (result *gosnmp.SnmpPacket, err error) {
	w.Context = ctx
	snmpRes := make(chan snmpResult)
	go func() {
		pkt, err := w.Get(oids)
		snmpRes <- snmpResult{pkt, err}
	}()
	select {
	case <-ctx.Done():
		if w.Conn != nil {
			w.Conn.Close()
		}
		return nil, fmt.Errorf("snmp get cancelled")
	case res := <-snmpRes:
		return res.pkt, res.err
	}
}

// GetNextWithCtx calls GetNext with a context
func (w *GoSNMPWrapper) GetNextWithCtx(ctx context.Context, oids []string) (result *gosnmp.SnmpPacket, err error) {
	w.Context = ctx
	snmpRes := make(chan snmpResult)
	go func() {
		pkt, err := w.GetNext(oids)
		snmpRes <- snmpResult{pkt, err}
	}()
	select {
	case <-ctx.Done():
		if w.Conn != nil {
			w.Conn.Close()
		}
		return nil, fmt.Errorf("snmp get next cancelled")
	case res := <-snmpRes:
		return res.pkt, res.err
	}
}

// GetBulkWithCtx calls GetBulk with a context
func (w *GoSNMPWrapper) GetBulkWithCtx(ctx context.Context, oids []string, nonRepeaters uint8, maxRepetitions uint32) (result *gosnmp.SnmpPacket, err error) {
	w.Context = ctx
	snmpRes := make(chan snmpResult)
	go func() {
		pkt, err := w.GetBulk(oids, nonRepeaters, maxRepetitions)
		snmpRes <- snmpResult{pkt, err}
	}()
	select {
	case <-ctx.Done():
		if w.Conn != nil {
			w.Conn.Close()
		}
		return nil, fmt.Errorf("snmp get bulk cancelled")
	case res := <-snmpRes:
		return res.pkt, res.err
	}
}

// WalkWithCtx calls Walk with a context
func (w *GoSNMPWrapper) WalkWithCtx(ctx context.Context, rootOid string, walkFn gosnmp.WalkFunc) error {
	w.Context = ctx
	errCh := make(chan error)
	go func() {
		errCh <- w.Walk(rootOid, walkFn)
	}()
	select {
	case <-ctx.Done():
		if w.Conn != nil {
			w.Conn.Close()
		}
		return fmt.Errorf("snmp walk cancelled")
	case err := <-errCh:
		return err
	}
	return nil
}

// BulkWalkWithCtx calls BulkWalk with a context
func (w *GoSNMPWrapper) BulkWalkWithCtx(ctx context.Context, rootOid string, walkFn gosnmp.WalkFunc) error {
	w.Context = ctx
	errCh := make(chan error)
	go func() {
		errCh <- w.BulkWalk(rootOid, walkFn)
	}()
	select {
	case <-ctx.Done():
		if w.Conn != nil {
			w.Conn.Close()
		}
		return fmt.Errorf("snmp bulk walk cancelled")
	case err := <-errCh:
		return err
	}
	return nil
}
