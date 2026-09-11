package main

import (
	"fmt"

	"erent/internal/config"
)

type apiConfiguration struct {
	runtime            config.Config
	oai                config.OIDCConfig
	upstream           config.UpstreamClientConfig
	oauthEncryptionKey []byte
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
	upstreamConfiguration, err := config.LoadUpstreamClientConfig()
	if err != nil {
		return apiConfiguration{}, fmt.Errorf("load upstream configuration: %w", err)
	}
	var encryptionKey []byte
	if oaiConfiguration.Enabled() {
		encryptionKey, err = config.LoadOAuthEncryptionKey()
		if err != nil {
			return apiConfiguration{}, err
		}
	}
	return apiConfiguration{
		runtime:            runtimeConfiguration,
		oai:                oaiConfiguration,
		upstream:           upstreamConfiguration,
		oauthEncryptionKey: encryptionKey,
	}, nil
}
