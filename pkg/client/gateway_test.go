/*
Copyright 2020 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

//go:generate mockgen -destination ./gateway_mock_test.go -package ${GOPACKAGE} github.com/hyperledger/fabric-protos-go/gateway GatewayClient

// WithClient uses the supplied client for the Gateway. Allows a stub implementation to be used for testing.
func WithClient(client gateway.GatewayClient) ConnectOption {
	return func(gateway *Gateway) error {
		gateway.client.grpcClient = client
		return nil
	}
}

// WithIdentity uses the supplied identity for the Gateway.
func WithIdentity(id identity.Identity) ConnectOption {
	return func(gateway *Gateway) error {
		gateway.signingID.id = id
		return nil
	}
}

func AssertNewTestGateway(t *testing.T, options ...ConnectOption) *Gateway {
	options = append([]ConnectOption{WithSign(TestCredentials.sign)}, options...)
	gateway, err := Connect(TestCredentials.identity, options...)
	require.NoError(t, err)

	return gateway
}

func TestGateway(t *testing.T) {
	id := TestCredentials.identity
	sign := TestCredentials.sign

	t.Run("Connect Gateway with no endpoint returns error", func(t *testing.T) {
		_, err := Connect(id, WithSign(sign))

		require.Error(t, err)
	})

	t.Run("Connect Gateway using existing gRPC client connection", func(t *testing.T) {
		var clientConnection *grpc.ClientConn
		gateway, err := Connect(id, WithSign(sign), WithClientConnection(clientConnection))

		require.NoError(t, err)
		require.NotNil(t, gateway)
	})

	t.Run("Close Gateway using existing gRPC client connection does not close connection", func(t *testing.T) {
		var clientConnection *grpc.ClientConn
		gateway, err := Connect(id, WithSign(sign), WithClientConnection(clientConnection))
		require.NoError(t, err)

		err = gateway.Close() // This would panic if clientConnection.Close() was called
		require.NoError(t, err)
	})

	t.Run("Connect Gateway with failing option returns error", func(t *testing.T) {
		expectedErr := errors.New("GATEWAY_OPTION_ERROR")
		badOption := func(gateway *Gateway) error {
			return expectedErr
		}
		_, actualErr := Connect(id, badOption)

		require.ErrorIs(t, actualErr, expectedErr)
	})

	t.Run("GetNetwork returns correctly named Network", func(t *testing.T) {
		networkName := "network"
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		gateway := AssertNewTestGateway(t, WithClient(mockClient))

		network := gateway.GetNetwork(networkName)

		require.NotNil(t, network)
		require.Equal(t, networkName, network.name)
	})

	t.Run("Identity returns connecting identity", func(t *testing.T) {
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		gateway := AssertNewTestGateway(t, WithIdentity(id), WithClient(mockClient))

		result := gateway.Identity()

		require.Equal(t, id, result)
	})
}
