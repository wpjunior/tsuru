// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func (s *S) prepareMultiCluster(c *check.C) (*kTesting.ClientWrapper, *kTesting.ClientWrapper, *kTesting.ClientWrapper) {
	cluster1 := &provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr1"},
		Default:     true,
		Provisioner: provisionerName,
		CustomData:  map[string]string{},
	}
	clusterClient1, err := NewClusterClient(cluster1)
	c.Assert(err, check.IsNil)
	client1 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		ClusterInterface:       clusterClient1,
	}
	clusterClient1.Interface = client1

	cluster2 := &provTypes.Cluster{
		Name:        "c2",
		Addresses:   []string{"https://clusteraddr2"},
		Pools:       []string{"pool2"},
		Provisioner: provisionerName,
		CustomData:  map[string]string{},
	}
	clusterClient2, err := NewClusterClient(cluster2)
	c.Assert(err, check.IsNil)
	client2 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		ClusterInterface:       clusterClient2,
	}
	clusterClient2.Interface = client2

	cluster3 := &provTypes.Cluster{
		Name:        "c3",
		Addresses:   []string{"https://clusteraddr3"},
		Pools:       []string{"pool3"},
		Provisioner: provisionerName,
		CustomData: map[string]string{
			disableNodeContainers: "true",
		},
	}
	clusterClient3, err := NewClusterClient(cluster2)
	c.Assert(err, check.IsNil)
	client3 := &kTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
		ClusterInterface:       clusterClient2,
	}
	clusterClient3.Interface = client3

	s.mockService.Cluster.OnFindByProvisioner = func(provName string) ([]provTypes.Cluster, error) {
		return []provTypes.Cluster{*cluster1, *cluster2, *cluster3}, nil
	}

	s.mockService.Cluster.OnFindByPool = func(provName, poolName string) (*provTypes.Cluster, error) {
		if poolName == "pool2" {
			return cluster2, nil
		}
		if poolName == "pool3" {
			return cluster3, nil
		}
		return cluster1, nil
	}

	ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
		if conf.Host == "https://clusteraddr1" {
			return client1, nil
		}
		if conf.Host == "https://clusteraddr3" {
			return client3, nil
		}
		return client2, nil
	}

	return client1, client2, client3
}
