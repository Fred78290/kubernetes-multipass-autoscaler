package main

import (
	"encoding/json"
	"fmt"
	"net/url"

	apiv1 "k8s.io/api/core/v1"
)

// NodeFromJSON deserialize a string to apiv1.Node
func nodeFromJSON(s string) (*apiv1.Node, error) {
	data := &apiv1.Node{}

	err := json.Unmarshal([]byte(s), &data)

	return data, err
}

func toJSON(v interface{}) string {
	if v == nil {
		return ""
	}

	b, _ := json.Marshal(v)

	return string(b)
}

func nodeGroupIDFromProviderID(serverIdentifier string, providerID string) (string, error) {
	var nodeIdentifier *url.URL
	var err error

	if nodeIdentifier, err = url.ParseRequestURI(providerID); err != nil {
		return "", err
	}

	if nodeIdentifier == nil {
		return "", fmt.Errorf(errCantDecodeNodeID, providerID)
	}

	if nodeIdentifier.Scheme != serverIdentifier {
		return "", fmt.Errorf(errWrongSchemeInProviderID, providerID, nodeIdentifier.Scheme)
	}

	if nodeIdentifier.Path != "object" && nodeIdentifier.Path != "/object" {
		return "", fmt.Errorf(errWrongPathInProviderID, providerID, nodeIdentifier.Path)
	}

	return nodeIdentifier.Hostname(), nil
}

func nodeNameFromProviderID(serverIdentifier string, providerID string) (string, error) {
	var nodeIdentifier *url.URL
	var err error

	if nodeIdentifier, err = url.ParseRequestURI(providerID); err != nil {
		return "", err
	}

	if nodeIdentifier == nil {
		return "", fmt.Errorf(errCantDecodeNodeID, providerID)
	}

	if nodeIdentifier.Scheme != serverIdentifier {
		return "", fmt.Errorf(errWrongSchemeInProviderID, providerID, nodeIdentifier.Scheme)
	}

	if nodeIdentifier.Path != "object" && nodeIdentifier.Path != "/object" {
		return "", fmt.Errorf(errWrongPathInProviderID, providerID, nodeIdentifier.Path)
	}

	return nodeIdentifier.Query().Get("name"), nil
}
