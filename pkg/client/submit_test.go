/*
Copyright 2020 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hyperledger/fabric-gateway/pkg/internal/test"
	"github.com/hyperledger/fabric-gateway/pkg/internal/util"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/runtime/protoiface"
)

func AssertMarshal(t *testing.T, message protoiface.MessageV1, msgAndArgs ...interface{}) []byte {
	bytes, err := util.Marshal(message)
	require.NoError(t, err, msgAndArgs...)
	return bytes
}

func AssertNewEndorseResponse(t *testing.T, result string, channelName string) *gateway.EndorseResponse {
	return &gateway.EndorseResponse{
		PreparedTransaction: &common.Envelope{
			Payload: AssertMarshal(t, &common.Payload{
				Header: &common.Header{
					ChannelHeader: AssertMarshal(t, &common.ChannelHeader{
						ChannelId: channelName,
					}),
				},
				Data: AssertMarshal(t, &peer.Transaction{
					Actions: []*peer.TransactionAction{
						{
							Payload: AssertMarshal(t, &peer.ChaincodeActionPayload{
								Action: &peer.ChaincodeEndorsedAction{
									ProposalResponsePayload: AssertMarshal(t, &peer.ProposalResponsePayload{
										Extension: AssertMarshal(t, &peer.ChaincodeAction{
											Response: &peer.Response{
												Payload: []byte(result),
											},
										}),
									}),
								},
							}),
						},
					},
				}),
			}),
		},
	}
}

func TestSubmitTransaction(t *testing.T) {
	newCommitStatusResponse := func(status peer.TxValidationCode, blockNumber uint64) *gateway.CommitStatusResponse {
		return &gateway.CommitStatusResponse{
			Result:      status,
			BlockNumber: blockNumber,
		}
	}

	t.Run("Returns endorse error", func(t *testing.T) {
		expected := NewStatusError(t, codes.Aborted, "ENDORSE_ERROR")
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(nil, expected)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))
		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")

		_, err = proposal.Endorse()

		require.Errorf(t, err, expected.Error(), "error message")
		require.Equal(t, status.Code(expected), status.Code(err), "status code")
		var actual *EndorseError
		require.ErrorAsf(t, err, &actual, "error type: %T", err)
		require.Equal(t, proposal.TransactionID(), actual.TransactionID, "transaction ID")
	})

	t.Run("Returns submit error", func(t *testing.T) {
		expected := NewStatusError(t, codes.Aborted, "SUBMIT_ERROR")
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, expected)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))
		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")
		transaction, err := proposal.Endorse()
		require.NoError(t, err, "Endorse")

		_, err = transaction.Submit()

		require.Errorf(t, err, expected.Error(), "error message")
		require.Equal(t, status.Code(expected), status.Code(err), "status code")
		var actual *SubmitError
		require.ErrorAsf(t, err, &actual, "error type: %T", err)
		require.Equal(t, proposal.TransactionID(), actual.TransactionID, "transaction ID")
	})

	t.Run("Returns commit status error", func(t *testing.T) {
		expected := NewStatusError(t, codes.Aborted, "COMMIT_ERROR")
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(nil, expected)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))
		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")
		transaction, err := proposal.Endorse()
		require.NoError(t, err, "Endorse")
		commit, err := transaction.Submit()
		require.NoError(t, err, "Submit")

		_, err = commit.Status()

		require.Errorf(t, err, expected.Error(), "error message")
		require.Equal(t, status.Code(expected), status.Code(err), "status code")
		var actual *CommitStatusError
		require.ErrorAsf(t, err, &actual, "error type: %T", err)
		require.Equal(t, proposal.TransactionID(), actual.TransactionID, "transaction ID")
	})

	t.Run("Returns result for committed transaction", func(t *testing.T) {
		expected := []byte("TRANSACTION_RESULT")
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		actual, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		require.Equal(t, expected, actual)
	})

	t.Run("Returns commit error for invalid commit status", func(t *testing.T) {
		expectedError := peer.TxValidationCode_name[int32(peer.TxValidationCode_MVCC_READ_CONFLICT)]
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_MVCC_READ_CONFLICT, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, err := contract.SubmitTransaction("transaction")

		require.Errorf(t, err, expectedError, "error message")
		var actual *CommitError
		require.ErrorAsf(t, err, &actual, "error type: %T", err)
		require.NotEmpty(t, actual.TransactionID, "transaction ID")
		require.Equal(t, peer.TxValidationCode_MVCC_READ_CONFLICT, actual.Code, "validation code")
	})

	t.Run("Includes channel name in proposal", func(t *testing.T) {
		var actual string
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actual = test.AssertUnmarshallChannelheader(t, in.ProposedTransaction).ChannelId
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		expected := contract.channelName
		require.Equal(t, expected, actual)
	})

	t.Run("Includes chaincode name in proposal", func(t *testing.T) {
		var actual string
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actual = test.AssertUnmarshallInvocationSpec(t, in.ProposedTransaction).ChaincodeSpec.ChaincodeId.Name
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		expected := contract.chaincodeName
		require.Equal(t, expected, actual)
	})

	t.Run("Includes transaction name in proposal for default contract", func(t *testing.T) {
		var args [][]byte
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				args = test.AssertUnmarshallInvocationSpec(t, in.ProposedTransaction).ChaincodeSpec.Input.Args
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		expected := "TRANSACTION_NAME"
		_, err := contract.SubmitTransaction(expected)
		require.NoError(t, err)

		actual := string(args[0])
		require.Equal(t, expected, actual)
	})

	t.Run("Includes transaction name in proposal for named contract", func(t *testing.T) {
		var args [][]byte
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				args = test.AssertUnmarshallInvocationSpec(t, in.ProposedTransaction).ChaincodeSpec.Input.Args
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContractWithName(t, "chaincode", "CONTRACT_NAME", WithClient(mockClient))

		_, err := contract.SubmitTransaction("TRANSACTION_NAME")
		require.NoError(t, err)

		actual := string(args[0])
		expected := "CONTRACT_NAME:TRANSACTION_NAME"
		require.Equal(t, expected, actual)
	})

	t.Run("Includes arguments in proposal", func(t *testing.T) {
		var args [][]byte
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				args = test.AssertUnmarshallInvocationSpec(t, in.ProposedTransaction).ChaincodeSpec.Input.Args
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		expected := []string{"one", "two", "three"}
		_, err := contract.SubmitTransaction("transaction", expected...)
		require.NoError(t, err)

		actual := bytesAsStrings(args[1:])
		require.EqualValues(t, expected, actual)
	})

	t.Run("Includes channel name in proposed transaction", func(t *testing.T) {
		var actual string
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actual = in.ChannelId
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		expected := contract.channelName
		require.Equal(t, expected, actual)
	})

	t.Run("Includes transaction ID in proposed transaction", func(t *testing.T) {
		var actual string

		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actual = test.AssertUnmarshallChannelheader(t, in.ProposedTransaction).TxId
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")
		_, err = proposal.Endorse()
		require.NoError(t, err, "Endorse")

		require.Equal(t, proposal.TransactionID(), actual)
	})

	t.Run("Includes transaction ID in endorse request", func(t *testing.T) {
		var actual string

		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actual = in.TransactionId
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")
		_, err = proposal.Endorse()
		require.NoError(t, err, "Endorse")

		require.Equal(t, proposal.TransactionID(), actual)
	})

	t.Run("Includes channel name in commit status request", func(t *testing.T) {
		var actual string
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.SignedCommitStatusRequest, _ ...grpc.CallOption) {
				request := &gateway.CommitStatusRequest{}
				test.AssertUnmarshall(t, in.Request, request)
				actual = request.ChannelId
			}).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		expected := contract.channelName
		require.Equal(t, expected, actual)
	})

	t.Run("Includes transaction ID in commit status request", func(t *testing.T) {
		var actual string
		var expected string
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				expected = test.AssertUnmarshallChannelheader(t, in.ProposedTransaction).TxId
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.SignedCommitStatusRequest, _ ...grpc.CallOption) {
				request := &gateway.CommitStatusRequest{}
				test.AssertUnmarshall(t, in.Request, request)
				actual = request.TransactionId
			}).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		require.Equal(t, expected, actual)
	})

	t.Run("Uses signer for endorse", func(t *testing.T) {
		var actual []byte
		expected := []byte("MY_SIGNATURE")
		sign := func(digest []byte) ([]byte, error) {
			return expected, nil
		}
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actual = in.ProposedTransaction.Signature
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient), WithSign(sign))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		require.EqualValues(t, expected, actual)
	})

	t.Run("Uses signer for submit", func(t *testing.T) {
		var actual []byte
		expected := []byte("MY_SIGNATURE")
		sign := func(digest []byte) ([]byte, error) {
			return expected, nil
		}
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.SubmitRequest, _ ...grpc.CallOption) {
				actual = in.PreparedTransaction.Signature
			}).
			Return(nil, nil).
			Times(1)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient), WithSign(sign))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		require.EqualValues(t, expected, actual)
	})

	t.Run("Sends private data with submit", func(t *testing.T) {
		var actualOrgs []string
		expectedOrgs := []string{"MY_ORG"}
		var actualPrice []byte
		expectedPrice := []byte("3000")
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actualOrgs = in.EndorsingOrganizations
				transient := test.AssertUnmarshallProposalPayload(t, in.ProposedTransaction).TransientMap
				actualPrice = transient["price"]
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		privateData := map[string][]byte{
			"price": []byte("3000"),
		}

		_, err := contract.Submit("transaction", WithTransient(privateData), WithEndorsingOrganizations("MY_ORG"))
		require.NoError(t, err)

		require.EqualValues(t, expectedOrgs, actualOrgs)
		require.EqualValues(t, expectedPrice, actualPrice)
	})

	t.Run("Uses signer for commit status", func(t *testing.T) {
		var actual []byte
		expected := []byte("MY_SIGNATURE")
		sign := func(digest []byte) ([]byte, error) {
			return expected, nil
		}
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, in *gateway.SignedCommitStatusRequest, _ ...grpc.CallOption) {
				actual = in.Signature
			}).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient), WithSign(sign))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		require.EqualValues(t, expected, actual)
	})

	t.Run("Uses hash", func(t *testing.T) {
		var actual [][]byte
		digest := []byte("MY_DIGEST")
		sign := func(digest []byte) ([]byte, error) {
			actual = append(actual, digest)
			return digest, nil
		}
		hash := func(message []byte) []byte {
			return digest
		}
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient), WithSign(sign), WithHash(hash))

		_, err := contract.SubmitTransaction("transaction")
		require.NoError(t, err)

		expected := [][]byte{digest, digest, digest}
		require.EqualValues(t, expected, actual)
	})

	t.Run("Commit returns transaction status", func(t *testing.T) {
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_MVCC_READ_CONFLICT, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, commit, err := contract.SubmitAsync("transaction")
		require.NoError(t, err)

		status, err := commit.Status()
		require.NoError(t, err)

		require.Equal(t, peer.TxValidationCode_MVCC_READ_CONFLICT, status.Code)
	})

	t.Run("Commit returns successful for successful transaction", func(t *testing.T) {
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, commit, err := contract.SubmitAsync("transaction")
		require.NoError(t, err, "submit")

		status, err := commit.Status()
		require.NoError(t, err, "commit status")

		require.True(t, status.Successful)
	})

	t.Run("Commit returns unsuccessful for failed transaction", func(t *testing.T) {
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_MVCC_READ_CONFLICT, 1), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, commit, err := contract.SubmitAsync("transaction")
		require.NoError(t, err, "submit")

		status, err := commit.Status()
		require.NoError(t, err, "commit status")

		require.False(t, status.Successful)
	})

	t.Run("Commit returns block number", func(t *testing.T) {
		expectedBlockNumber := uint64(101)
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_MVCC_READ_CONFLICT, expectedBlockNumber), nil)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		_, commit, err := contract.SubmitAsync("transaction")
		require.NoError(t, err, "submit")

		status, err := commit.Status()
		require.NoError(t, err, "commit status")

		require.Equal(t, expectedBlockNumber, status.BlockNumber)
	})

	t.Run("Uses specified context for endorse", func(t *testing.T) {
		var actual context.Context

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, _ *gateway.EndorseRequest, _ ...grpc.CallOption) {
				actual = ctx
			}).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")

		_, err = proposal.EndorseWithContext(ctx)
		require.NoError(t, err, "Endorse")

		require.Nil(t, actual.Err(), "context not done before explicit cancel")
		cancel()
		require.NotNil(t, actual.Err(), "context done after explicit cancel")
	})

	t.Run("Uses default context for endorse", func(t *testing.T) {
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ *gateway.EndorseRequest, _ ...grpc.CallOption) (*gateway.EndorseResponse, error) {
				select {
				case <-time.After(1 * time.Second):
					return AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil
				case <-ctx.Done(): // Zero timeout context should cancel immediately, selecting this case
					return nil, ctx.Err()
				}
			})
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil).
			AnyTimes()

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient), WithEndorseTimeout(0))

		_, err := contract.Submit("transaction")

		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("Uses specified context for submit", func(t *testing.T) {
		var actual context.Context

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, _ *gateway.SubmitRequest, _ ...grpc.CallOption) {
				actual = ctx
			}).
			Return(nil, nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")

		transaction, err := proposal.Endorse()
		require.NoError(t, err, "Endorse")

		_, err = transaction.SubmitWithContext(ctx)
		require.NoError(t, err, "Submit")

		require.Nil(t, actual.Err(), "context not done before explicit cancel")
		cancel()
		require.NotNil(t, actual.Err(), "context done after explicit cancel")
	})

	t.Run("Uses default context for submit", func(t *testing.T) {
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ *gateway.SubmitRequest, _ ...grpc.CallOption) (*gateway.SubmitResponse, error) {
				select {
				case <-time.After(1 * time.Second):
					return nil, nil
				case <-ctx.Done(): // Zero timeout context should cancel immediately, selecting this case
					return nil, ctx.Err()
				}
			})
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil).
			AnyTimes()

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient), WithSubmitTimeout(0))

		_, err := contract.Submit("transaction")

		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("Uses specified context for commit status", func(t *testing.T) {
		var actual context.Context

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Do(func(ctx context.Context, _ *gateway.SignedCommitStatusRequest, _ ...grpc.CallOption) {
				actual = ctx
			}).
			Return(newCommitStatusResponse(peer.TxValidationCode_MVCC_READ_CONFLICT, 101), nil).
			Times(1)

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient))

		proposal, err := contract.NewProposal("transaction")
		require.NoError(t, err, "NewProposal")

		transaction, err := proposal.Endorse()
		require.NoError(t, err, "Endorse")

		commit, err := transaction.Submit()
		require.NoError(t, err, "Submit")

		_, err = commit.StatusWithContext(ctx)
		require.NoError(t, err, "CommitStatus")

		require.Nil(t, actual.Err(), "context not done before explicit cancel")
		cancel()
		require.NotNil(t, actual.Err(), "context done after explicit cancel")
	})

	t.Run("Uses default context for commit status", func(t *testing.T) {
		mockClient := NewMockGatewayClient(gomock.NewController(t))
		mockClient.EXPECT().Endorse(gomock.Any(), gomock.Any()).
			Return(AssertNewEndorseResponse(t, "TRANSACTION_RESULT", "network"), nil)
		mockClient.EXPECT().Submit(gomock.Any(), gomock.Any()).
			Return(nil, nil)
		mockClient.EXPECT().CommitStatus(gomock.Any(), gomock.Any()).
			Return(newCommitStatusResponse(peer.TxValidationCode_VALID, 1), nil).
			DoAndReturn(func(ctx context.Context, _ *gateway.SignedCommitStatusRequest, _ ...grpc.CallOption) (*gateway.CommitStatusResponse, error) {
				select {
				case <-time.After(1 * time.Second):
					return nil, nil
				case <-ctx.Done(): // Zero timeout context should cancel immediately, selecting this case
					return nil, ctx.Err()
				}
			})

		contract := AssertNewTestContract(t, "chaincode", WithClient(mockClient), WithCommitStatusTimeout(0))

		_, err := contract.Submit("transaction")

		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
