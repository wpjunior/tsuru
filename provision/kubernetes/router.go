// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/kubernetes/routers"
	routerTypes "github.com/tsuru/tsuru/types/router"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	swapLabel = "tsuru.io/swapped-with"
)

var (
	RouterKindLoadbalancer = "loadbalancer"
	RouterKindIngress      = "ingress"
)

func (p *kubernetesProvisioner) CreateRouter(opts provision.RouterOptions) error {
	kubeRouter, err := getKubeRouter(opts)
	if err != nil {
		return err
	}
	return kubeRouter.CreateRouter(opts)
}

func (p *kubernetesProvisioner) UpdateRouter(opts provision.RouterOptions) error {
	kubeRouter, err := getKubeRouter(opts)
	if err != nil {
		return err
	}
	return kubeRouter.UpdateRouter(opts)
}

func getKubeRouter(opts provision.RouterOptions) (routers.KubeRouter, error) {
	labels, err := getLabelSet(opts.RouterConfig, "labels")
	if err != nil {
		return nil, err
	}

	optsToLabel, err := getMap(opts.RouterConfig, "opts-to-label")
	if err != nil {
		return nil, err
	}

	optsToLabelDoc, err := getMap(opts.RouterConfig, "opts-to-label-doc")
	if err != nil {
		return nil, err
	}

	poolLabels, err := getPoolLabels(opts.RouterConfig)
	if err != nil {
		return nil, err
	}

	clusterClient, err := clusterForPool(opts.App.GetPool())
	if err != nil {
		return nil, err
	}

	ns, err := clusterClient.AppNamespace(opts.App)
	if err != nil {
		return nil, err
	}

	if opts.RouterKind == RouterKindLoadbalancer {
		return &routers.LBService{
			BaseService: &routers.BaseService{
				Namespace: ns,
				Client:    clusterClient.Interface,
				Labels:    labels,
			},
			OptsAsLabels:     optsToLabel,
			OptsAsLabelsDocs: optsToLabelDoc,
			PoolLabels:       poolLabels,
		}, nil
	}

	return nil, errors.New("no valid routerType")
}

func getMap(routerConfig routerTypes.ConfigGetter, key string) (map[string]string, error) {
	m := map[string]string{}
	rawValue, err := routerConfig.Get(key)
	if _, isKeyNotFound := errors.Cause(err).(config.ErrKeyNotFound); isKeyNotFound {
		return m, nil
	}

	if err != nil {
		return nil, err
	}

	strValue, ok := rawValue.(string)
	if ok {
		pairs := strings.Split(strValue, ",")

		for _, pair := range pairs {
			l := strings.Split(pair, "=")
			if len(l) != 2 {
				return nil, fmt.Errorf("invalid map: %s", l)
			}
			key := strings.TrimSpace(l[0])
			value := strings.TrimSpace(l[1])
			m[key] = value
		}

		return m, nil
	}

	return m, nil
}

func getLabelSet(routerConfig routerTypes.ConfigGetter, key string) (labels.Set, error) {
	rawValue, err := routerConfig.GetString(key)
	if _, isKeyNotFound := errors.Cause(err).(config.ErrKeyNotFound); isKeyNotFound {
		return labels.Set{}, nil
	}

	if err != nil {
		return nil, err
	}

	return labels.ConvertSelectorToLabelsMap(rawValue)
}

func getPoolLabels(routerConfig routerTypes.ConfigGetter) (map[string]labels.Set, error) {
	poolLabels := map[string]labels.Set{}
	rawValue, err := routerConfig.Get("pool-labels")
	if _, isKeyNotFound := errors.Cause(err).(config.ErrKeyNotFound); isKeyNotFound {
		return poolLabels, nil
	}

	if err != nil {
		return nil, err
	}

	rootMap, ok := rawValue.(map[interface{}]interface{})
	if !ok {
		return nil, errors.New("config pool-labels is not a map[interface{}]interface{}")
	}

	for key, value := range rootMap {
		keyStr := key.(string)

		set, err := labels.ConvertSelectorToLabelsMap(value.(string))
		if err != nil {
			return nil, err
		}
		poolLabels[keyStr] = set
	}

	return poolLabels, nil
}
