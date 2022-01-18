// Copyright 2021 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ixgnmi

import (
	"golang.org/x/net/context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/patrickmn/go-cache"
	"google.golang.org/protobuf/testing/protocmp"
	"github.com/openconfig/ygot/uexampleoc"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/gnmi/errdiff"
	"github.com/openconfig/ondatra/internal/ixconfig"
	"github.com/openconfig/ondatra/telemetry"

	gpb "github.com/openconfig/gnmi/proto/gnmi"
)

func TestSubscribe(t *testing.T) {
	var stubStats []map[string]ygot.ValidatedGoStruct
	readStatsFn = func(_ context.Context, _ *Client, _ string, captions []string) (ygot.GoStruct, error) {
		var data ygot.ValidatedGoStruct = &uexampleoc.Device{}
		if len(stubStats) == 0 {
			return data, nil
		}
		stub := stubStats[0]
		stubStats = stubStats[1:]
		for _, cap := range captions {
			t, ok := stub[cap]
			if !ok {
				continue
			}
			var err error
			data, err = ygot.MergeStructs(data, t)
			if err != nil {
				return nil, err
			}
		}
		return data, nil
	}

	ctx := context.Background()

	devWithAttr1 := &telemetry.Device{}
	devWithAttr1.GetOrCreateNetworkInstance("foo").GetOrCreateProtocol(telemetry.PolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "1").
		GetOrCreateBgp().GetOrCreateRib().GetOrCreateAttrSet(1)

	devWithAttr2 := &telemetry.Device{}
	devWithAttr2.GetOrCreateNetworkInstance("foo").GetOrCreateProtocol(telemetry.PolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "1").
		GetOrCreateBgp().GetOrCreateRib().GetOrCreateAttrSet(2)

	devWithPartial := &telemetry.Device{}
	devWithPartial.GetOrCreateNetworkInstance("foo")

	tests := []struct {
		name        string
		mode        gpb.SubscriptionList_Mode
		path        *gpb.Path
		stats       []map[string]ygot.ValidatedGoStruct
		learnedInfo []*telemetry.Device
		want        []*gpb.SubscribeResponse
	}{{
		name: "no such path",
		path: &gpb.Path{Elem: []*gpb.PathElem{{Name: "bogus"}}},
		mode: gpb.SubscriptionList_ONCE,
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
		},
	}, {
		name: "components once",
		mode: gpb.SubscriptionList_ONCE,
		path: &gpb.Path{
			Origin: "openconfig",
			Elem:   []*gpb.PathElem{{Name: "components"}},
		},
		stats: []map[string]ygot.ValidatedGoStruct{{
			portCPUStatsCaption: func() ygot.ValidatedGoStruct {
				root := &uexampleoc.Device{}
				root.GetOrCreateComponents().GetOrCreateComponent("fakeComp")
				return root
			}(),
		}},
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Path: &gpb.Path{Elem: []*gpb.PathElem{
						{Name: "components"},
						{Name: "component", Key: map[string]string{"name": "fakeComp"}},
						{Name: "name"},
					}},
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_StringVal{"fakeComp"}}},
				},
			}}},
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
		},
	}, {
		name: "components stream",
		mode: gpb.SubscriptionList_STREAM,
		path: &gpb.Path{
			Origin: "openconfig",
			Elem:   []*gpb.PathElem{{Name: "components"}},
		},
		stats: []map[string]ygot.ValidatedGoStruct{{
			portCPUStatsCaption: func() ygot.ValidatedGoStruct {
				root := &uexampleoc.Device{}
				root.GetOrCreateComponents().GetOrCreateComponent("fakeComp")
				return root
			}(),
		}, {
			portCPUStatsCaption: func() ygot.ValidatedGoStruct {
				root := &uexampleoc.Device{}
				root.GetOrCreateComponents().GetOrCreateComponent("fakeComp2")
				return root
			}(),
		}},
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Path: &gpb.Path{Elem: []*gpb.PathElem{
						{Name: "components"},
						{Name: "component", Key: map[string]string{"name": "fakeComp"}},
						{Name: "name"},
					}},
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_StringVal{"fakeComp"}}},
				},
			}}},
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Path: &gpb.Path{Elem: []*gpb.PathElem{
						{Name: "components"},
						{Name: "component", Key: map[string]string{"name": "fakeComp2"}},
						{Name: "name"},
					}},
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_StringVal{"fakeComp2"}}},
				},
			}}},
		},
	}, {
		name: "interfaces once",
		mode: gpb.SubscriptionList_ONCE,
		path: &gpb.Path{
			Origin: "openconfig",
			Elem:   []*gpb.PathElem{{Name: "interfaces"}},
		},
		stats: []map[string]ygot.ValidatedGoStruct{{
			portStatsCaption: func() ygot.ValidatedGoStruct {
				root := &uexampleoc.Device{}
				root.GetOrCreateInterfaces().GetOrCreateInterface("fakeIntf")
				return root
			}(),
		}},
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Path: &gpb.Path{Elem: []*gpb.PathElem{
						{Name: "interfaces"},
						{Name: "interface", Key: map[string]string{"name": "fakeIntf"}},
						{Name: "name"},
					}},
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_StringVal{"fakeIntf"}}},
				},
			}}},
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
		},
	}, {
		name: "interfaces stream",
		mode: gpb.SubscriptionList_STREAM,
		path: &gpb.Path{
			Origin: "openconfig",
			Elem:   []*gpb.PathElem{{Name: "interfaces"}},
		},
		stats: []map[string]ygot.ValidatedGoStruct{{
			portStatsCaption: func() ygot.ValidatedGoStruct {
				root := &uexampleoc.Device{}
				root.GetOrCreateInterfaces().GetOrCreateInterface("fakeIntf")
				return root
			}(),
		}, {
			portStatsCaption: func() ygot.ValidatedGoStruct {
				root := &uexampleoc.Device{}
				root.GetOrCreateInterfaces().GetOrCreateInterface("fakeIntf2")
				return root
			}(),
		}},
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Path: &gpb.Path{Elem: []*gpb.PathElem{
						{Name: "interfaces"},
						{Name: "interface", Key: map[string]string{"name": "fakeIntf"}},
						{Name: "name"},
					}},
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_StringVal{"fakeIntf"}}},
				},
			}}},
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Path: &gpb.Path{Elem: []*gpb.PathElem{
						{Name: "interfaces"},
						{Name: "interface", Key: map[string]string{"name": "fakeIntf2"}},
						{Name: "name"},
					}},
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_StringVal{"fakeIntf2"}}},
				},
			}}},
		},
	}, {
		name: "bgp rib once",
		mode: gpb.SubscriptionList_ONCE,
		learnedInfo: []*telemetry.Device{
			devWithAttr1,
		},
		path: &gpb.Path{
			Origin: "openconfig",
			Elem: []*gpb.PathElem{
				{Name: "network-instances"},
				{Name: "network-instance", Key: map[string]string{"name": "foo"}},
				{Name: "protocols"},
				{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
				{Name: "bgp"},
				{Name: "rib"},
				{Name: "attr-sets"},
				{Name: "attr-set", Key: map[string]string{"index": "1"}},
				{Name: "index"},
			},
		},
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_UintVal{1}},
					Path: &gpb.Path{
						Elem: []*gpb.PathElem{
							{Name: "network-instances"},
							{Name: "network-instance", Key: map[string]string{"name": "foo"}},
							{Name: "protocols"},
							{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
							{Name: "bgp"},
							{Name: "rib"},
							{Name: "attr-sets"},
							{Name: "attr-set", Key: map[string]string{"index": "1"}},
							{Name: "index"},
						},
					},
				}},
			}}},
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
		},
	}, {
		name: "bgp rib streaming",
		mode: gpb.SubscriptionList_STREAM,
		learnedInfo: []*telemetry.Device{
			devWithAttr1,
			devWithAttr2,
		},
		path: &gpb.Path{
			Origin: "openconfig",
			Elem: []*gpb.PathElem{
				{Name: "network-instances"},
				{Name: "network-instance", Key: map[string]string{"name": "foo"}},
				{Name: "protocols"},
				{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
				{Name: "bgp"},
				{Name: "rib"},
				{Name: "attr-sets"},
				{Name: "attr-set", Key: map[string]string{"index": "*"}},
				{Name: "state"},
				{Name: "index"},
			},
		},
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_UintVal{1}},
					Path: &gpb.Path{
						Elem: []*gpb.PathElem{
							{Name: "network-instances"},
							{Name: "network-instance", Key: map[string]string{"name": "foo"}},
							{Name: "protocols"},
							{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
							{Name: "bgp"},
							{Name: "rib"},
							{Name: "attr-sets"},
							{Name: "attr-set", Key: map[string]string{"index": "1"}},
							{Name: "state"},
							{Name: "index"},
						},
					},
				}},
			}}},
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_UintVal{2}},
					Path: &gpb.Path{
						Elem: []*gpb.PathElem{
							{Name: "network-instances"},
							{Name: "network-instance", Key: map[string]string{"name": "foo"}},
							{Name: "protocols"},
							{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
							{Name: "bgp"},
							{Name: "rib"},
							{Name: "attr-sets"},
							{Name: "attr-set", Key: map[string]string{"index": "2"}},
							{Name: "state"},
							{Name: "index"},
						},
					},
				}},
			}}},
		},
	}, {
		name: "bgp rib streaming with no data",
		mode: gpb.SubscriptionList_STREAM,
		learnedInfo: []*telemetry.Device{
			nil,
			nil,
			devWithAttr1,
		},
		path: &gpb.Path{
			Origin: "openconfig",
			Elem: []*gpb.PathElem{
				{Name: "network-instances"},
				{Name: "network-instance", Key: map[string]string{"name": "foo"}},
				{Name: "protocols"},
				{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
				{Name: "bgp"},
				{Name: "rib"},
				{Name: "attr-sets"},
				{Name: "attr-set", Key: map[string]string{"index": "*"}},
				{Name: "state"},
				{Name: "index"},
			},
		},
		want: []*gpb.SubscribeResponse{
			{Response: &gpb.SubscribeResponse_SyncResponse{true}},
			{Response: &gpb.SubscribeResponse_Update{&gpb.Notification{
				Prefix: &gpb.Path{Origin: "openconfig", Target: "fakeIxia"},
				Update: []*gpb.Update{{
					Val: &gpb.TypedValue{Value: &gpb.TypedValue_UintVal{1}},
					Path: &gpb.Path{
						Elem: []*gpb.PathElem{
							{Name: "network-instances"},
							{Name: "network-instance", Key: map[string]string{"name": "foo"}},
							{Name: "protocols"},
							{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
							{Name: "bgp"},
							{Name: "rib"},
							{Name: "attr-sets"},
							{Name: "attr-set", Key: map[string]string{"index": "1"}},
							{Name: "state"},
							{Name: "index"},
						},
					},
				}},
			}}},
		},
	}}

	/*
	 * TODO: Add test cases for the flows root. Unfortunately today that
	 * requires dependency on the Ondatra Telemetry API generation, which could
	 * create a circulate dependency back on this module. If and when the
	 * Telemetry API is an independent library, add these test cases.
	 */

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(ctx, "fakeIxia", nil, nil)
			if err != nil {
				t.Fatalf("NewClient() got unexpected error: %v", err)
			}

			prefixToReader[ribOCPath] = &prefixReader{read: func(_ context.Context, _ *Client, p *gpb.Path) ([]*gpb.Notification, error) {
				if len(tt.learnedInfo) == 0 {
					return nil, nil
				}
				defer func() { tt.learnedInfo = tt.learnedInfo[1:] }()
				if tt.learnedInfo[0] == nil {
					return nil, nil
				}
				ns, err := ygot.TogNMINotifications(tt.learnedInfo[0], time.Now().UnixNano(), ygot.GNMINotificationsConfig{UsePathElem: true})
				if err != nil {
					t.Fatalf("failed to create gNMI notifications: %v", err)
				}
				return ns, nil
			}}

			sub, err := client.Subscribe(ctx)
			if err != nil {
				t.Fatalf("Subscribe() got unexpected error: %v", err)
			}
			stubStats = tt.stats
			req := &gpb.SubscribeRequest{Request: &gpb.SubscribeRequest_Subscribe{&gpb.SubscriptionList{
				Mode:         tt.mode,
				Subscription: []*gpb.Subscription{{Path: tt.path}}},
			}}
			if err := sub.Send(req); err != nil {
				t.Fatalf("Send(%v) got unexpected error: %v", req, err)
			}

			for _, want := range tt.want {
				// Flush the freshness cache to force the data to be repopulated.
				client.fresh.Flush()
				got, err := sub.Recv()
				if err != nil {
					t.Fatalf("Recv() got unexpected error: %v", err)
				}
				// Zero out the timestamps when comparing.
				if up, ok := got.GetResponse().(*gpb.SubscribeResponse_Update); ok {
					up.Update.Timestamp = 0
				}
				if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
					t.Errorf("Recv() got unexpected response diff (-want,+got): %s", diff)
				} else {
					t.Logf("Recv() correctly got %v", got)
				}
			}
			if len(stubStats) > 0 {
				t.Errorf("Unused stub statistics: %v", stubStats)
			}
			if len(tt.learnedInfo) > 0 {
				t.Errorf("Unused stub statistics: %v", stubStats)
			}
		})
	}
}

type fakeCfgClient struct {
	getRsps   map[string]string
	getErrs   map[string]error
	postErrs  map[string]error
	pushErrs  []error
	cfg       *ixconfig.Ixnetwork
	cfgErr    error
	updateErr error
}

func (f *fakeCfgClient) Session() session {
	return f
}

func (f *fakeCfgClient) Get(_ context.Context, p string, v interface{}) error {
	if f.getRsps[p] != "" {
		json.Unmarshal([]byte(f.getRsps[p]), v)
	}
	return f.getErrs[p]
}

func (f *fakeCfgClient) Post(_ context.Context, p string, _, _ interface{}) error {
	return f.postErrs[p]
}

func (f *fakeCfgClient) LastImportedConfig() *ixconfig.Ixnetwork {
	return f.cfg
}

func (f *fakeCfgClient) GetNode(ctx context.Context, n ixconfig.IxiaCfgNode, v interface{}) error {
	return f.Get(ctx, n.GetRestID(), v)
}

func (f *fakeCfgClient) UpdateIDs(context.Context, *ixconfig.Ixnetwork, ...ixconfig.IxiaCfgNode) error {
	return f.updateErr
}

func TestRIBFromIxia(t *testing.T) {
	fullCfg := &ixconfig.Ixnetwork{
		Topology: []*ixconfig.Topology{{
			DeviceGroup: []*ixconfig.TopologyDeviceGroup{{
				Name: ixconfig.String("Device Group on eth0"),
				Ethernet: []*ixconfig.TopologyEthernet{{
					Ipv4: []*ixconfig.TopologyIpv4{{
						BgpIpv4Peer: []*ixconfig.TopologyBgpIpv4Peer{{
							Name:   ixconfig.String("fake v4"),
							DutIp:  ixconfig.MultivalueStr("localhost"),
							RestID: "/api/v1/sessions/0/topology/1/deviceGroup/1/ethernet/1/ipv4/1/bgpIpv4Peer/1",
						}},
					}},
					Ipv6: []*ixconfig.TopologyIpv6{{
						BgpIpv6Peer: []*ixconfig.TopologyBgpIpv6Peer{{
							Name:   ixconfig.String("fake v6"),
							DutIp:  ixconfig.MultivalueStr("::1"),
							RestID: "/api/v1/sessions/0/topology/1/deviceGroup/1/ethernet/1/ipv6/1/bgpIpv6Peer/1",
						}},
					}},
				}},
			}},
		}},
	}

	tests := []struct {
		desc      string
		ipv4      bool
		neighbor  string
		intfName  string
		cfg       *ixconfig.Ixnetwork
		postErr   map[string]error
		getErr    map[string]error
		getRsps   map[string]string
		cfgErr    error
		updateErr error
		want      *table
		wantErr   string
	}{{
		desc:    "get config error",
		wantErr: "no IxNetwork config found",
	}, {
		desc:      "update ID error",
		wantErr:   "failed to update IDs",
		updateErr: errors.New("fake"),
		cfg:       fullCfg,
	}, {
		desc:    "not in cache",
		wantErr: `no peer "" on interface ""`,
		cfg:     fullCfg,
	}, {
		desc:     "not in cache no ethernet",
		intfName: "eth0",
		wantErr:  `no peer "" on interface "eth0"`,
		cfg: &ixconfig.Ixnetwork{
			Topology: []*ixconfig.Topology{{
				DeviceGroup: []*ixconfig.TopologyDeviceGroup{{
					Name: ixconfig.String("Device Group on eth0"),
				}},
			}},
		},
	}, {
		desc:     "not in cache no ipv4",
		intfName: "eth0",
		wantErr:  `no peer "" on interface "eth0"`,
		cfg: &ixconfig.Ixnetwork{
			Topology: []*ixconfig.Topology{{
				DeviceGroup: []*ixconfig.TopologyDeviceGroup{{
					Name: ixconfig.String("Device Group on eth0"),
					Ethernet: []*ixconfig.TopologyEthernet{{
						Ipv6: []*ixconfig.TopologyIpv6{{}},
					}},
				}},
			}},
		},
	}, {
		desc:     "not in cache no ipv6",
		intfName: "eth0",
		wantErr:  `no peer "" on interface "eth0"`,
		cfg: &ixconfig.Ixnetwork{
			Topology: []*ixconfig.Topology{{
				DeviceGroup: []*ixconfig.TopologyDeviceGroup{{
					Name: ixconfig.String("Device Group on eth0"),
					Ethernet: []*ixconfig.TopologyEthernet{{
						Ipv4: []*ixconfig.TopologyIpv4{{}},
					}},
				}},
			}},
		},
	}, {
		desc:     "run op error",
		wantErr:  "failed to run op",
		intfName: "eth0",
		neighbor: "localhost",
		ipv4:     true,
		postErr: map[string]error{
			"topology/deviceGroup/ethernet/ipv4/bgpIpv4Peer/operations/getAllLearnedInfo": errors.New("v4 fake"),
		},
		cfg: fullCfg,
	}, {
		desc:     "get error",
		wantErr:  "failed to get learned info",
		intfName: "eth0",
		neighbor: "localhost",
		ipv4:     true,
		getErr: map[string]error{
			"/api/v1/sessions/0/topology/1/deviceGroup/1/ethernet/1/ipv4/1/bgpIpv4Peer/1/learnedInfo/1/table/1": errors.New("fake"),
		},
		cfg: fullCfg,
	}, {
		desc:     "success ipv4",
		intfName: "eth0",
		neighbor: "localhost",
		ipv4:     true,
		getRsps: map[string]string{
			"/api/v1/sessions/0/topology/1/deviceGroup/1/ethernet/1/ipv4/1/bgpIpv4Peer/1/learnedInfo/1/table/1": `{"id": 1}`,
		},
		want: &table{
			ID: 1,
		},
		cfg: fullCfg,
	}, {
		desc:     "success ipv6",
		intfName: "eth0",
		neighbor: "::1",
		getRsps: map[string]string{
			"/api/v1/sessions/0/topology/1/deviceGroup/1/ethernet/1/ipv6/1/bgpIpv6Peer/1/learnedInfo/1/table/1": `{"id": 1}`,
		},
		want: &table{
			ID: 1,
		},
		cfg: fullCfg,
	}}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c := &Client{
				client: &fakeCfgClient{
					postErrs:  tt.postErr,
					getErrs:   tt.getErr,
					getRsps:   tt.getRsps,
					cfg:       tt.cfg,
					updateErr: tt.updateErr,
				},
			}
			pi := peerInfo{
				neighbor: tt.neighbor,
				intf:     tt.intfName,
				isIPV4:   tt.ipv4,
			}
			got, err := c.ribFromIxia(context.Background(), pi)
			if d := errdiff.Substring(err, tt.wantErr); d != "" {
				t.Fatalf("unexpected error diff\n%s", d)
			}
			if err != nil {
				return
			}
			if d := cmp.Diff(tt.want, got); d != "" {
				t.Errorf("unexpected table diff (-want +got)\n%s", d)
			}
		})
	}
}

func createSampleDev(t testing.TB, deletes ...*gpb.Path) *gpb.Notification {
	dev := &telemetry.Device{}
	rib := dev.GetOrCreateNetworkInstance("foo").
		GetOrCreateProtocol(telemetry.PolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "1").
		GetOrCreateBgp().GetOrCreateRib()

	route := rib.GetOrCreateAfiSafi(telemetry.BgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST).
		GetOrCreateIpv4Unicast().GetOrCreateNeighbor("localhost").
		GetOrCreateAdjRibInPre().GetOrCreateRoute("192.168.1.1/0", 0)

	route.AttrIndex = ygot.Uint64(0)
	route.CommunityIndex = ygot.Uint64(0)

	rib.GetOrCreateCommunity(0)
	attr := rib.GetOrCreateAttrSet(0)
	attr.Origin = telemetry.BgpTypes_BgpOriginAttrType_IGP
	attr.NextHop = ygot.String("")
	attr.Aigp = ygot.Uint64(0)
	attr.LocalPref = ygot.Uint32(0)
	attr.Med = ygot.Uint32(0)

	ns, err := ygot.TogNMINotifications(dev, 0, ygot.GNMINotificationsConfig{UsePathElem: true})
	if err != nil {
		t.Fatal("error creating notifications")
	}
	ns[0].Delete = deletes
	return ns[0]
}

func TestPathToOCRIB(t *testing.T) {
	indexPath := &gpb.Path{
		Elem: []*gpb.PathElem{
			{Name: "network-instances"},
			{Name: "network-instance", Key: map[string]string{"name": "foo"}},
			{Name: "protocols"},
			{Name: "protocol", Key: map[string]string{"identifier": "BGP", "name": "1"}},
			{Name: "bgp"},
			{Name: "rib"},
			{Name: "afi-safis"},
			{Name: "afi-safi"},
			{Name: "ipv4-unicast"},
			{Name: "neighbors"},
			{Name: "neighbor", Key: map[string]string{"neighbor-address": "localhost"}},
			{Name: "adj-rib-in-pre"},
			{Name: "routes"},
			{Name: "route"},
			{Name: "state"},
		},
	}

	tests := []struct {
		desc      string
		path      *gpb.Path
		want      *gpb.Notification
		rib       *table
		ribErr    error
		initCache map[string]interface{}
		wantErr   string
	}{{
		desc:    "nil path",
		wantErr: "failed to get schema path",
	}, {
		desc: "read attr set with nothing cached",
		path: &gpb.Path{
			Elem: []*gpb.PathElem{
				{Name: "network-instances"},
				{Name: "network-instance"},
				{Name: "protocols"},
				{Name: "protocol"},
				{Name: "bgp"},
				{Name: "rib"},
				{Name: "attr-sets"},
				{Name: "attr-set"},
				{Name: "index"},
			},
		},
		wantErr: "need to read the attr index",
	}, {
		desc: "read attr set with cached values",
		path: &gpb.Path{
			Elem: []*gpb.PathElem{
				{Name: "network-instances"},
				{Name: "network-instance"},
				{Name: "protocols"},
				{Name: "protocol"},
				{Name: "bgp"},
				{Name: "rib"},
				{Name: "attr-sets"},
				{Name: "attr-set"},
				{Name: "index"},
			},
		},
		initCache: map[string]interface{}{
			peerInfoCacheKey: peerInfo{},
		},
	}, {
		desc: "unknown paths",
		path: &gpb.Path{
			Elem: []*gpb.PathElem{
				{Name: "network-instances"},
			},
		},
	}, {
		desc: "rib cache hit with same peer",
		path: indexPath,
		initCache: map[string]interface{}{
			ribOCPath: true,
			peerInfoCacheKey: peerInfo{
				protocolName: "1",
				intf:         "foo",
				neighbor:     "localhost",
				isIPV4:       true,
			},
		},
	}, {
		desc:    "get rib error",
		path:    indexPath,
		ribErr:  errors.New("foo"),
		wantErr: "failed to read Ixia table",
	}, {
		desc:    "unmarshal error",
		path:    indexPath,
		wantErr: "failed to unmarshal Ixia table",
		rib: &table{
			Columns: []string{"Prefix Length"},
			Values:  [][]string{{"foo"}},
		},
	}, {
		desc: "uncached success",
		path: indexPath,
		rib: &table{
			Columns: []string{"IPv4 Prefix ", "Origin"},
			Values:  [][]string{{"192.168.1.1", "IGP"}},
		},
		want:      createSampleDev(t),
		initCache: map[string]interface{}{},
	}, {
		desc: "cached success",
		path: indexPath,
		rib: &table{
			Columns: []string{"IPv4 Prefix ", "Origin"},
			Values:  [][]string{{"192.168.1.1", "IGP"}},
		},
		want: createSampleDev(t,
			&gpb.Path{Elem: []*gpb.PathElem{{Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "fake"}}, {Name: "state"}, {Name: "name"}}},
			&gpb.Path{Elem: []*gpb.PathElem{{Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "fake"}}, {Name: "name"}}},
		),
		// This is a contrived example where there is garbage data in the cache, used to verify delete notifications are created.
		// In normal usage, only BGP RIB info could be in the cache.
		initCache: map[string]interface{}{
			oldRibCacheKey: &telemetry.Device{
				Interface: map[string]*telemetry.Interface{
					"fake": &telemetry.Interface{
						Name: ygot.String("fake"),
					},
				},
			},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c := &Client{
				fresh: cache.New(30*time.Second, cache.NoExpiration),
			}
			for k, v := range tt.initCache {
				c.fresh.Set(k, v, -1)
			}
			ribFromIxiaFn = func(*Client, context.Context, peerInfo) (*table, error) {
				return tt.rib, tt.ribErr
			}

			got, gotErr := c.pathToOCRIB(context.Background(), tt.path)
			if diff := errdiff.Substring(gotErr, tt.wantErr); diff != "" {
				t.Errorf("pathToOCRIB() got unexpected error diff\n%s", diff)
			}
			if gotErr != nil {
				return
			}
			if got != nil {
				got.Timestamp = 0
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform(), protocmp.SortRepeatedFields(&gpb.Notification{}, "update")); diff != "" {
				t.Errorf("pathToOCRIB() got unexpected response diff (-want,+got)\n%s", diff)
			}
		})
	}
}