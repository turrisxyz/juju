// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	serviceAccountSecretCADataKey = "ca.crt"
	serviceAccountSecretTokenKey  = "token"
)

func GetControllerProxy(
	name,
	apiHost string,
	configI core.ConfigMapInterface,
	saI core.ServiceAccountInterface,
	secretI core.SecretInterface,
) (*Proxier, error) {
	cm, err := configI.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("controller proxy config %s", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	config := ControllerProxyConfig{}
	if err := json.Unmarshal([]byte(cm.Data[ProxyConfigMapKey]), &config); err != nil {
		return nil, errors.Trace(err)
	}

	sa, err := saI.Get(context.TODO(), config.Name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("controller proxy service account for %s", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if len(sa.Secrets) == 0 {
		return nil, fmt.Errorf("no secret created for service account %q", sa.GetName())
	}

	sec, err := secretI.Get(context.TODO(), sa.Secrets[0].Name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("could not get proxy service account secret: %s", sa.Secrets[0].Name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	proxierConfig := ProxierConfig{
		APIHost:             apiHost,
		CAData:              string(sec.Data[serviceAccountSecretCADataKey]),
		Namespace:           config.Namespace,
		RemotePort:          config.RemotePort,
		Service:             config.TargetService,
		ServiceAccountToken: string(sec.Data[serviceAccountSecretTokenKey]),
	}

	return NewProxier(proxierConfig), nil
}

func HasControllerProxy(
	name string,
	configI core.ConfigMapInterface,
) (bool, error) {
	_, err := configI.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}
