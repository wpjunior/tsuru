// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	check "gopkg.in/check.v1"
)

func (s *S) Test_RoutableProvisioner_CreateRouter(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	routerConfig := router.ConfigGetterFromData(map[string]interface{}{
		"labels":            "puppet.io/datacenter=prod_cme",
		"opts-to-label":     "lb-name=csccm.cloudprovider.io/loadbalancer-name",
		"opts-to-label-doc": "lb-name=\"Custom domain name used on VIP creation.\"",
		"pool-labels": map[string]interface{}{
			"dev":     "puppet.io/datacenter=dev",
			"staging": "puppet.io/datacenter=staging",
		},
	})
	err := s.p.CreateRouter(provision.RouterOptions{
		App:          a,
		RouterKind:   RouterKindLoadbalancer,
		RouterConfig: routerConfig,
		InstanceOpts: map[string]string{},
	})
	c.Assert(err, check.IsNil)
}
