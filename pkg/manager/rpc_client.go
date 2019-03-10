package manager

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/rpc"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skycoin/skywire/pkg/cipher"
	"github.com/skycoin/skywire/pkg/node"
	"github.com/skycoin/skywire/pkg/routing"
	"github.com/skycoin/skywire/pkg/transport"
)

// RPCClient provides methods to call an RPC Server.
// It implements node.RPCClient
type rpcClient struct {
	client *rpc.Client
	prefix string
}

// NewRPCClient creates a new RPCClient.
func NewRPCClient(rc *rpc.Client, prefix string) node.RPCClient {
	return &rpcClient{client: rc, prefix: prefix}
}

// Call calls the internal rpc.Client with the serviceMethod arg prefixed.
func (rc *rpcClient) Call(method string, args, reply interface{}) error {
	return rc.client.Call(rc.prefix+"."+method, args, reply)
}

// Summary calls Summary.
func (rc *rpcClient) Summary() (*node.Summary, error) {
	out := new(node.Summary)
	err := rc.Call("Summary", &struct{}{}, out)
	return out, err
}

// Apps calls Apps.
func (rc *rpcClient) Apps() ([]*node.AppState, error) {
	states := make([]*node.AppState, 0)
	err := rc.Call("Apps", &struct{}{}, &states)
	return states, err
}

// StartApp calls StartApp.
func (rc *rpcClient) StartApp(appName string) error {
	return rc.Call("StartApp", &appName, &struct{}{})
}

// StopApp calls StopApp.
func (rc *rpcClient) StopApp(appName string) error {
	return rc.Call("StopApp", &appName, &struct{}{})
}

// SetAutoStart calls SetAutoStart.
func (rc *rpcClient) SetAutoStart(appName string, autostart bool) error {
	return rc.Call("SetAutoStart", &node.SetAutoStartIn{
		AppName:   appName,
		AutoStart: autostart,
	}, &struct{}{})
}

// TransportTypes calls TransportTypes.
func (rc *rpcClient) TransportTypes() ([]string, error) {
	var types []string
	err := rc.Call("TransportTypes", &struct{}{}, &types)
	return types, err
}

// Transports calls Transports.
func (rc *rpcClient) Transports(types []string, pks []cipher.PubKey, logs bool) ([]*node.TransportSummary, error) {
	var transports []*node.TransportSummary
	err := rc.Call("Transports", &node.TransportsIn{
		FilterTypes:   types,
		FilterPubKeys: pks,
		ShowLogs:      logs,
	}, &transports)
	return transports, err
}

// Transport calls Transport.
func (rc *rpcClient) Transport(tid uuid.UUID) (*node.TransportSummary, error) {
	var summary node.TransportSummary
	err := rc.Call("Transport", &tid, &summary)
	return &summary, err
}

// AddTransport calls AddTransport.
func (rc *rpcClient) AddTransport(remote cipher.PubKey, tpType string, public bool, timeout time.Duration) (*node.TransportSummary, error) {
	var summary node.TransportSummary
	err := rc.Call("AddTransport", &node.AddTransportIn{
		RemotePK: remote,
		TpType:   tpType,
		Public:   public,
		Timeout:  timeout,
	}, &summary)
	return &summary, err
}

// RemoveTransport calls RemoveTransport.
func (rc *rpcClient) RemoveTransport(tid uuid.UUID) error {
	return rc.Call("RemoveTransport", &tid, &struct{}{})
}

// RoutingRules calls RoutingRules.
func (rc *rpcClient) RoutingRules() ([]*node.RoutingEntry, error) {
	var entries []*node.RoutingEntry
	err := rc.Call("RoutingRules", &struct{}{}, &entries)
	return entries, err
}

// RoutingRule calls RoutingRule.
func (rc *rpcClient) RoutingRule(key routing.RouteID) (routing.Rule, error) {
	var rule routing.Rule
	err := rc.Call("RoutingRule", &key, &rule)
	return rule, err
}

// AddRoutingRule calls AddRoutingRule.
func (rc *rpcClient) AddRoutingRule(rule routing.Rule) (routing.RouteID, error) {
	var tid routing.RouteID
	err := rc.Call("AddRoutingRule", &rule, &tid)
	return tid, err
}

// SetRoutingRule calls SetRoutingRule.
func (rc *rpcClient) SetRoutingRule(key routing.RouteID, rule routing.Rule) error {
	return rc.Call("SetRoutingRule", &node.RoutingEntry{Key: key, Value: rule}, &struct{}{})
}

// RemoveRoutingRule calls RemoveRoutingRule.
func (rc *rpcClient) RemoveRoutingRule(key routing.RouteID) error {
	return rc.Call("RemoveRoutingRule", &key, &struct{}{})
}

// MockRPCClient mocks node.RPCClient.
type mockRPCClient struct {
	s       *node.Summary
	tpTypes []string
	rt      routing.Table
	sync.RWMutex
}

// NewMockRPCClient creates a new mock RPCClient.
func NewMockRPCClient(r *rand.Rand, maxTps int, maxRules int) node.RPCClient {
	types := []string{"messaging", "native"}
	localPK, _ := cipher.GenerateKeyPair()
	tps := make([]*node.TransportSummary, r.Intn(maxTps+1))
	for i := range tps {
		remotePK, _ := cipher.GenerateKeyPair()
		tps[i] = &node.TransportSummary{
			ID:     uuid.New(),
			Local:  localPK,
			Remote: remotePK,
			Type:   types[r.Int()%len(types)],
			Log:    new(transport.LogEntry),
		}
	}
	rt := routing.InMemoryRoutingTable()
	ruleExp := time.Now().Add(time.Hour * 24)
	for i := 0; i < r.Intn(maxRules+1); i++ {
		remotePK, _ := cipher.GenerateKeyPair()
		var rule routing.Rule
		if r.Int()%2 == 0 {
			var lpRaw, rpRaw [2]byte
			r.Read(lpRaw[:])
			r.Read(rpRaw[:])
			lp := binary.BigEndian.Uint16(lpRaw[:])
			rp := binary.BigEndian.Uint16(rpRaw[:])
			rule = routing.AppRule(ruleExp, routing.RouteID(r.Uint32()), remotePK, rp, lp)
		} else {
			rule = routing.ForwardRule(ruleExp, routing.RouteID(r.Uint32()), uuid.New())
		}
		if _, err := rt.AddRule(rule); err != nil {
			panic(err)
		}
	}
	return &mockRPCClient{
		s: &node.Summary{
			PubKey: localPK,
			Apps: []*node.AppState{
				{Name: "foo.v1.0", AutoStart: false, Port: 10},
				{Name: "bar.v2.0", AutoStart: false, Port: 20},
			},
			Transports: tps,
		},
		tpTypes: types,
		rt:      rt,
	}
}

func (mc *mockRPCClient) do(write bool, f func() error) error {
	if write {
		mc.Lock()
		defer mc.Unlock()
	} else {
		mc.RLock()
		defer mc.RUnlock()
	}
	return f()
}

// Summary implements RPCClient.
func (mc *mockRPCClient) Summary() (*node.Summary, error) {
	var out node.Summary
	err := mc.do(false, func() error {
		out.PubKey = mc.s.PubKey
		for _, app := range mc.s.Apps {
			out.Apps = append(out.Apps, &(*app))
		}
		for _, tp := range mc.s.Transports {
			out.Transports = append(out.Transports, &(*tp))
		}
		return nil
	})
	return &out, err
}

// Apps implements RPCClient.
func (mc *mockRPCClient) Apps() ([]*node.AppState, error) {
	var apps []*node.AppState
	err := mc.do(false, func() error {
		for _, app := range mc.s.Apps {
			apps = append(apps, &(*app))
		}
		return nil
	})
	return apps, err
}

// StartApp implements RPCClient.
func (*mockRPCClient) StartApp(string) error {
	return nil
}

// StopApp implements RPCClient.
func (*mockRPCClient) StopApp(string) error {
	return nil
}

// SetAutoStart implements RPCClient.
func (mc *mockRPCClient) SetAutoStart(appName string, autostart bool) error {
	return mc.do(true, func() error {
		for _, app := range mc.s.Apps {
			if app.Name == appName {
				app.AutoStart = autostart
				return nil
			}
		}
		return fmt.Errorf("app of name '%s' does not exist", appName)
	})
}

// TransportTypes implements RPCClient.
func (mc *mockRPCClient) TransportTypes() ([]string, error) {
	return mc.tpTypes, nil
}

// Transports implements RPCClient.
func (mc *mockRPCClient) Transports(types []string, pks []cipher.PubKey, logs bool) ([]*node.TransportSummary, error) {
	var summaries []*node.TransportSummary
	err := mc.do(false, func() error {
		for _, tp := range mc.s.Transports {
			if types != nil {
				for _, reqT := range types {
					if tp.Type == reqT {
						goto TypeOK
					}
				}
				continue
			}
		TypeOK:
			if pks != nil {
				for _, reqPK := range pks {
					if tp.Remote == reqPK || tp.Local == reqPK {
						goto PubKeyOK
					}
				}
				continue
			}
		PubKeyOK:
			if !logs {
				temp := *tp
				temp.Log = nil
				summaries = append(summaries, &temp)
			} else {
				summaries = append(summaries, &(*tp))
			}
		}
		return nil
	})
	return summaries, err
}

// Transport implements RPCClient.
func (mc *mockRPCClient) Transport(tid uuid.UUID) (*node.TransportSummary, error) {
	var summary node.TransportSummary
	err := mc.do(false, func() error {
		for _, tp := range mc.s.Transports {
			if tp.ID == tid {
				summary = *tp
				return nil
			}
		}
		return fmt.Errorf("transport of id '%s' is not found", tid)
	})
	return &summary, err
}

// AddTransport implements RPCClient.
func (mc *mockRPCClient) AddTransport(remote cipher.PubKey, tpType string, public bool, _ time.Duration) (*node.TransportSummary, error) {
	summary := &node.TransportSummary{
		ID:     uuid.New(),
		Local:  mc.s.PubKey,
		Remote: remote,
		Type:   tpType,
		Log:    new(transport.LogEntry),
	}
	return summary, mc.do(true, func() error {
		mc.s.Transports = append(mc.s.Transports, summary)
		return nil
	})
}

// RemoveTransport implements RPCClient.
func (mc *mockRPCClient) RemoveTransport(tid uuid.UUID) error {
	return mc.do(true, func() error {
		for i, tp := range mc.s.Transports {
			if tp.ID == tid {
				mc.s.Transports = append(mc.s.Transports[:i], mc.s.Transports[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("transport of id '%s' is not found", tid)
	})
}

// RoutingRules implements RPCClient.
func (mc *mockRPCClient) RoutingRules() ([]*node.RoutingEntry, error) {
	var entries []*node.RoutingEntry
	err := mc.rt.RangeRules(func(routeID routing.RouteID, rule routing.Rule) (next bool) {
		entries = append(entries, &node.RoutingEntry{Key: routeID, Value: rule})
		return true
	})
	return entries, err
}

// RoutingRule implements RPCClient.
func (mc *mockRPCClient) RoutingRule(key routing.RouteID) (routing.Rule, error) {
	return mc.rt.Rule(key)
}

// AddRoutingRule implements RPCClient.
func (mc *mockRPCClient) AddRoutingRule(rule routing.Rule) (routing.RouteID, error) {
	return mc.rt.AddRule(rule)
}

// SetRoutingRule implements RPCClient.
func (mc *mockRPCClient) SetRoutingRule(key routing.RouteID, rule routing.Rule) error {
	return mc.rt.SetRule(key, rule)
}

// RemoveRoutingRule implements RPCClient.
func (mc *mockRPCClient) RemoveRoutingRule(key routing.RouteID) error {
	return mc.rt.DeleteRules(key)
}