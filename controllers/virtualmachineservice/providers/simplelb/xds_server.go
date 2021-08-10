// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package simplelb

import (
	"context"
	"fmt"
	"net"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type XdsServer struct {
	snapshotCache cache.SnapshotCache
	log           logr.Logger
}

func NewXdsServer(mgr manager.Manager, logger logr.Logger) *XdsServer {
	x := &XdsServer{
		snapshotCache: cache.NewSnapshotCache(false, cache.IDHash{}, nil),
		log:           logger,
	}
	_ = mgr.Add(x) // nothing can go wrong (we don't inject stuff)
	return x
}

func (x *XdsServer) Start(ctx context.Context) error {
	server := xds.NewServer(ctx, x.snapshotCache, nil)
	grpcServer := grpc.NewServer()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", XdsNodePort))
	if err != nil {
		return err
	}

	envoy_api_v2.RegisterEndpointDiscoveryServiceServer(grpcServer, server)

	go func() {
		<-ctx.Done()
		grpcServer.Stop()
	}()

	return grpcServer.Serve(lis)
}

func (x *XdsServer) UpdateEndpoints(svc *corev1.Service, eps *corev1.Endpoints) error {
	clusters := make([]cache.Resource, len(svc.Spec.Ports))
	endpoints := make([]cache.Resource, len(svc.Spec.Ports))
	for i, svcPort := range svc.Spec.Ports {
		clusters[i] = cluster(svcPort)
		endpoints[i] = clusterEndpoints(svcPort, eps.Subsets)
	}

	nodeID := nodeID(svc)
	snapshot := cache.NewSnapshot(eps.ResourceVersion, endpoints, clusters, nil, nil, nil)

	x.log.V(5).Info("setting xds snapshot", "nodeID", nodeID, "snapshot", snapshot)
	return x.snapshotCache.SetSnapshot(nodeID, snapshot)
}

func nodeID(svc *corev1.Service) string {
	return types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}.String()
}

func clusterName(svcPort corev1.ServicePort) string {
	if svcPort.Name == "" {
		protocol := string(svcPort.Protocol)
		if protocol == "" {
			protocol = string(corev1.ProtocolTCP)
		}
		return fmt.Sprintf("%s-%v", protocol, svcPort.Port)
	}
	return svcPort.Name
}

func clusterEndpoints(svcPort corev1.ServicePort, subsets []corev1.EndpointSubset) *envoy_api_v2.ClusterLoadAssignment {
	var lbEndpoints []*envoy_api_v2_endpoint.LbEndpoint

	for _, subset := range subsets {
		for _, endpointPort := range subset.Ports {
			if endpointPort.Port != svcPort.TargetPort.IntVal {
				continue
			}
			for _, endpointAddress := range subset.Addresses {
				lbEndpoints = append(lbEndpoints, &envoy_api_v2_endpoint.LbEndpoint{
					HostIdentifier: &envoy_api_v2_endpoint.LbEndpoint_Endpoint{
						Endpoint: &envoy_api_v2_endpoint.Endpoint{
							Address: &envoy_api_v2_core.Address{
								Address: &envoy_api_v2_core.Address_SocketAddress{
									SocketAddress: &envoy_api_v2_core.SocketAddress{
										Protocol: envoy_api_v2_core.SocketAddress_TCP,
										Address:  endpointAddress.IP,
										PortSpecifier: &envoy_api_v2_core.SocketAddress_PortValue{
											PortValue: uint32(endpointPort.Port),
										},
									},
								},
							},
						},
					},
				})
			}
		}
	}

	return &envoy_api_v2.ClusterLoadAssignment{
		ClusterName: clusterName(svcPort),
		Endpoints: []*envoy_api_v2_endpoint.LocalityLbEndpoints{{
			LbEndpoints: lbEndpoints,
		}},
	}
}

func cluster(svcPort corev1.ServicePort) *envoy_api_v2.Cluster {
	return &envoy_api_v2.Cluster{
		Name: clusterName(svcPort),
		ClusterDiscoveryType: &envoy_api_v2.Cluster_Type{
			Type: envoy_api_v2.Cluster_EDS,
		},
	}
}
