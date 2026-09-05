package main

import (
	"fmt"

	"erent/internal/config"
)

type apiConfiguration struct {
	runtime  config.Config
	oai      config.OIDCConfig
	upstream config.UpstreamClientConfig
}

func loadAPIConfiguration() (apiConfiguration, error) {
	runtimeConfiguration, err := config.Load()
	if err != nil {
		return apiConfiguration{}, err
	}
	oaiConfiguration, err := config.LoadOIDCConfig("oai")
	if err != nil {
		return apiConfiguration{}, fmt.Errorf("load OIDC configuration for oai: %w", err)
	}
	var upstreamConfiguration config.UpstreamClientConfig
	if oaiConfiguration.Enabled() {
		upstreamConfiguration, err = config.LoadUpstreamClientConfig()
		if err != nil {
			return apiConfiguration{}, fmt.Errorf("load upstream configuration: %w", err)
		}
	}
	return apiConfiguration{
		runtime:  runtimeConfiguration,
		oai:      oaiConfiguration,
		upstream: upstreamConfiguration,
	}, nil
}
