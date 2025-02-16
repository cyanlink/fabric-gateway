/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hyperledger/fabric-gateway/pkg/internal/util"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:generate mockgen -destination ./chaincodeevents_mock_test.go -package ${GOPACKAGE} github.com/hyperledger/fabric-protos-go/gateway Gateway_ChaincodeEventsClient

func TestChaincodeEvents(t *testing.T) {
	newChaincodeEventsResponse := func(events []*ChaincodeEvent) *gateway.ChaincodeEventsResponse {
		blockNumber := uint64(0)
		var peerEvents []*peer.ChaincodeEvent

		for _, event := range events {
			blockNumber = event.BlockNumber
			peerEvents = append(peerEvents, &peer.ChaincodeEvent{
				ChaincodeId: event.ChaincodeName,
				TxId:        event.TransactionID,
				EventName:   event.EventName,
				Payload:     event.Payload,
			})
		}

		return &gateway.ChaincodeEventsResponse{
			BlockNumber: blockNumber,
			Events:      peerEvents,
		}
	}

	t.Run("Returns connect error", func(t *testing.T) {
		expected := NewStatusError(t, codes.Aborted, "CHAINCODE_EVENTS_ERROR")
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().ChaincodeEvents(gomock.Any(), gomock.Any()).
			Return(nil, expected)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		network := AssertNewTestNetwork(t, "NETWORK", WithClient(mockClient))
		_, err := network.ChaincodeEvents(ctx, "CHAINCODE")

		require.Equal(t, status.Code(expected), status.Code(err), "status code")
		require.Errorf(t, err, expected.Error(), "error message")
	})

	t.Run("Sends valid request with default start position", func(t *testing.T) {
		controller := gomock.NewController(t)
		mockClient := NewMockGatewayClient(controller)
		mockEvents := NewMockGateway_ChaincodeEventsClient(controller)

		var actual *gateway.ChaincodeEventsRequest
		mockClient.EXPECT().ChaincodeEvents(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.SignedChaincodeEventsRequest, _ ...grpc.CallOption) {
				request := &gateway.ChaincodeEventsRequest{}
				err := util.Unmarshal(in.GetRequest(), request)
				require.NoError(t, err)
				actual = request
			}).
			Return(mockEvents, nil).
			Times(1)

		mockEvents.EXPECT().Recv().
			Return(nil, errors.New("fake")).
			AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		network := AssertNewTestNetwork(t, "NETWORK", WithClient(mockClient))
		_, err := network.ChaincodeEvents(ctx, "CHAINCODE")
		require.NoError(t, err)

		creator, err := network.signingID.Creator()
		require.NoError(t, err)

		expected := &gateway.ChaincodeEventsRequest{
			ChannelId:   "NETWORK",
			ChaincodeId: "CHAINCODE",
			Identity:    creator,
			StartPosition: &orderer.SeekPosition{
				Type: &orderer.SeekPosition_NextCommit{
					NextCommit: &orderer.SeekNextCommit{},
				},
			},
		}
		require.True(t, util.ProtoEqual(expected, actual), "Expected %v, got %v", expected, actual)
	})

	t.Run("Sends valid request with specified start block number", func(t *testing.T) {
		controller := gomock.NewController(t)
		mockClient := NewMockGatewayClient(controller)
		mockEvents := NewMockGateway_ChaincodeEventsClient(controller)

		var actual *gateway.ChaincodeEventsRequest
		mockClient.EXPECT().ChaincodeEvents(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.SignedChaincodeEventsRequest, _ ...grpc.CallOption) {
				request := &gateway.ChaincodeEventsRequest{}
				err := util.Unmarshal(in.GetRequest(), request)
				require.NoError(t, err)
				actual = request
			}).
			Return(mockEvents, nil).
			Times(1)

		mockEvents.EXPECT().Recv().
			Return(nil, errors.New("fake")).
			AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		network := AssertNewTestNetwork(t, "NETWORK", WithClient(mockClient))
		_, err := network.ChaincodeEvents(ctx, "CHAINCODE", WithStartBlock(418))
		require.NoError(t, err)

		creator, err := network.signingID.Creator()
		require.NoError(t, err)

		expected := &gateway.ChaincodeEventsRequest{
			ChannelId:   "NETWORK",
			ChaincodeId: "CHAINCODE",
			Identity:    creator,
			StartPosition: &orderer.SeekPosition{
				Type: &orderer.SeekPosition_Specified{
					Specified: &orderer.SeekSpecified{
						Number: 418,
					},
				},
			},
		}
		require.True(t, util.ProtoEqual(expected, actual), "Expected %v, got %v", expected, actual)
	})

	t.Run("Defaults to next commit as start position", func(t *testing.T) {
		controller := gomock.NewController(t)
		mockClient := NewMockGatewayClient(controller)
		mockEvents := NewMockGateway_ChaincodeEventsClient(controller)

		var actual *gateway.ChaincodeEventsRequest
		mockClient.EXPECT().ChaincodeEvents(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.SignedChaincodeEventsRequest, _ ...grpc.CallOption) {
				request := &gateway.ChaincodeEventsRequest{}
				err := util.Unmarshal(in.GetRequest(), request)
				require.NoError(t, err)

				actual = &gateway.ChaincodeEventsRequest{
					ChannelId:   request.ChannelId,
					ChaincodeId: request.ChaincodeId,
				}
			}).
			Return(mockEvents, nil).
			Times(1)

		mockEvents.EXPECT().Recv().
			Return(nil, errors.New("fake")).
			AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		network := AssertNewTestNetwork(t, "NETWORK", WithClient(mockClient))
		_, err := network.ChaincodeEvents(ctx, "CHAINCODE")
		require.NoError(t, err)

		expected := &gateway.ChaincodeEventsRequest{
			ChannelId:   "NETWORK",
			ChaincodeId: "CHAINCODE",
		}
		require.True(t, util.ProtoEqual(expected, actual), "Expected %v, got %v", expected, actual)
	})

	t.Run("Closes event channel on receive error", func(t *testing.T) {
		controller := gomock.NewController(t)
		mockClient := NewMockGatewayClient(controller)
		mockEvents := NewMockGateway_ChaincodeEventsClient(controller)

		mockClient.EXPECT().ChaincodeEvents(gomock.Any(), gomock.Any()).
			Return(mockEvents, nil)

		mockEvents.EXPECT().Recv().
			Return(nil, errors.New("fake")).
			AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		network := AssertNewTestNetwork(t, "NETWORK", WithClient(mockClient))
		receive, err := network.ChaincodeEvents(ctx, "CHAINCODE")
		require.NoError(t, err)

		actual, ok := <-receive

		require.False(t, ok, "Expected event listening to be cancelled, got %v", actual)
	})

	t.Run("Receives events", func(t *testing.T) {
		controller := gomock.NewController(t)
		mockClient := NewMockGatewayClient(controller)
		mockEvents := NewMockGateway_ChaincodeEventsClient(controller)

		mockClient.EXPECT().ChaincodeEvents(gomock.Any(), gomock.Any()).
			Return(mockEvents, nil)

		expected := []*ChaincodeEvent{
			{
				BlockNumber:   1,
				ChaincodeName: "CHAINCODE",
				EventName:     "EVENT_1",
				Payload:       []byte("PAYLOAD_1"),
				TransactionID: "TRANSACTION_ID_1",
			},
			{
				BlockNumber:   1,
				ChaincodeName: "CHAINCODE",
				EventName:     "EVENT_2",
				Payload:       []byte("PAYLOAD_2"),
				TransactionID: "TRANSACTION_ID_2",
			},
			{
				BlockNumber:   2,
				ChaincodeName: "CHAINCODE",
				EventName:     "EVENT_3",
				Payload:       []byte("PAYLOAD_3"),
				TransactionID: "TRANSACTION_ID_3",
			},
		}

		responses := []*gateway.ChaincodeEventsResponse{
			newChaincodeEventsResponse(expected[0:2]),
			newChaincodeEventsResponse(expected[2:]),
		}
		responseIndex := 0
		mockEvents.EXPECT().Recv().
			DoAndReturn(func() (*gateway.ChaincodeEventsResponse, error) {
				if responseIndex >= len(responses) {
					return nil, errors.New("fake")
				}
				response := responses[responseIndex]
				responseIndex++
				return response, nil
			}).
			AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		network := AssertNewTestNetwork(t, "NETWORK", WithClient(mockClient))
		receive, err := network.ChaincodeEvents(ctx, "CHAINCODE")
		require.NoError(t, err)

		for _, event := range expected {
			actual := <-receive
			require.EqualValues(t, event, actual)
		}
	})
}
