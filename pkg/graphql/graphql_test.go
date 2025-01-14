// VulcanizeDB
// Copyright © 2020 Vulcanize

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.

// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package graphql_test

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/statediff"
	"github.com/ethereum/go-ethereum/statediff/indexer"
	"github.com/ethereum/go-ethereum/statediff/indexer/node"
	"github.com/ethereum/go-ethereum/statediff/indexer/postgres"
	sdtypes "github.com/ethereum/go-ethereum/statediff/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vulcanize/ipld-eth-server/pkg/eth"
	"github.com/vulcanize/ipld-eth-server/pkg/eth/test_helpers"
	"github.com/vulcanize/ipld-eth-server/pkg/graphql"
	ethServerShared "github.com/vulcanize/ipld-eth-server/pkg/shared"
)

// SetupDB is use to setup a db for watcher tests
func SetupDB() (*postgres.DB, error) {
	port, _ := strconv.Atoi(os.Getenv("DATABASE_PORT"))
	uri := postgres.DbConnectionString(postgres.ConnectionParams{
		User:     os.Getenv("DATABASE_USER"),
		Password: os.Getenv("DATABASE_PASSWORD"),
		Hostname: os.Getenv("DATABASE_HOSTNAME"),
		Name:     os.Getenv("DATABASE_NAME"),
		Port:     port,
	})
	return postgres.NewDB(uri, postgres.ConnectionConfig{}, node.Info{})
}

var _ = Describe("GraphQL", func() {
	const (
		gqlEndPoint = "127.0.0.1:8083"
	)
	var (
		randomAddr      = common.HexToAddress("0x1C3ab14BBaD3D99F4203bd7a11aCB94882050E6f")
		randomHash      = crypto.Keccak256Hash(randomAddr.Bytes())
		blocks          []*types.Block
		receipts        []types.Receipts
		chain           *core.BlockChain
		db              *postgres.DB
		blockHashes     []common.Hash
		backend         *eth.Backend
		graphQLServer   *graphql.Service
		chainConfig     = params.TestChainConfig
		mockTD          = big.NewInt(1337)
		client          = graphql.NewClient(fmt.Sprintf("http://%s/graphql", gqlEndPoint))
		ctx             = context.Background()
		blockHash       common.Hash
		contractAddress common.Address
	)

	It("test init", func() {
		var err error
		db, err = SetupDB()
		Expect(err).ToNot(HaveOccurred())

		transformer, err := indexer.NewStateDiffIndexer(chainConfig, db)
		Expect(err).ToNot(HaveOccurred())
		backend, err = eth.NewEthBackend(db, &eth.Config{
			ChainConfig: chainConfig,
			VMConfig:    vm.Config{},
			RPCGasCap:   big.NewInt(10000000000),
			GroupCacheConfig: &ethServerShared.GroupCacheConfig{
				StateDB: ethServerShared.GroupConfig{
					Name:                   "graphql_test",
					CacheSizeInMB:          8,
					CacheExpiryInMins:      60,
					LogStatsIntervalInSecs: 0,
				},
			},
		})
		Expect(err).ToNot(HaveOccurred())

		// make the test blockchain (and state)
		blocks, receipts, chain = test_helpers.MakeChain(5, test_helpers.Genesis, test_helpers.TestChainGen)
		params := statediff.Params{
			IntermediateStateNodes:   true,
			IntermediateStorageNodes: true,
		}

		// iterate over the blocks, generating statediff payloads, and transforming the data into Postgres
		builder := statediff.NewBuilder(chain.StateCache())
		for i, block := range blocks {
			blockHashes = append(blockHashes, block.Hash())
			var args statediff.Args
			var rcts types.Receipts
			if i == 0 {
				args = statediff.Args{
					OldStateRoot: common.Hash{},
					NewStateRoot: block.Root(),
					BlockNumber:  block.Number(),
					BlockHash:    block.Hash(),
				}
			} else {
				args = statediff.Args{
					OldStateRoot: blocks[i-1].Root(),
					NewStateRoot: block.Root(),
					BlockNumber:  block.Number(),
					BlockHash:    block.Hash(),
				}
				rcts = receipts[i-1]
			}

			var diff statediff.StateObject
			diff, err = builder.BuildStateDiffObject(args, params)
			Expect(err).ToNot(HaveOccurred())

			tx, err := transformer.PushBlock(block, rcts, mockTD)
			Expect(err).ToNot(HaveOccurred())

			for _, node := range diff.Nodes {
				err = transformer.PushStateNode(tx, node)
				Expect(err).ToNot(HaveOccurred())
			}

			err = tx.Close(err)
			Expect(err).ToNot(HaveOccurred())
		}

		// Insert some non-canonical data into the database so that we test our ability to discern canonicity
		indexAndPublisher, err := indexer.NewStateDiffIndexer(chainConfig, db)
		Expect(err).ToNot(HaveOccurred())

		blockHash = test_helpers.MockBlock.Hash()
		contractAddress = test_helpers.ContractAddr

		tx, err := indexAndPublisher.PushBlock(test_helpers.MockBlock, test_helpers.MockReceipts, test_helpers.MockBlock.Difficulty())
		Expect(err).ToNot(HaveOccurred())

		err = tx.Close(err)
		Expect(err).ToNot(HaveOccurred())

		// The non-canonical header has a child
		tx, err = indexAndPublisher.PushBlock(test_helpers.MockChild, test_helpers.MockReceipts, test_helpers.MockChild.Difficulty())
		Expect(err).ToNot(HaveOccurred())

		ccHash := sdtypes.CodeAndCodeHash{
			Hash: test_helpers.CodeHash,
			Code: test_helpers.ContractCode,
		}

		err = indexAndPublisher.PushCodeAndCodeHash(tx, ccHash)
		Expect(err).ToNot(HaveOccurred())

		err = tx.Close(err)
		Expect(err).ToNot(HaveOccurred())

		graphQLServer, err = graphql.New(backend, gqlEndPoint, nil, []string{"*"}, rpc.HTTPTimeouts{})
		Expect(err).ToNot(HaveOccurred())

		err = graphQLServer.Start(nil)
		Expect(err).ToNot(HaveOccurred())
	})

	defer It("test teardown", func() {
		err := graphQLServer.Stop()
		Expect(err).ToNot(HaveOccurred())
		eth.TearDownDB(db)
		chain.Stop()
	})

	Describe("eth_getLogs", func() {
		It("Retrieves logs that matches the provided blockHash and contract address", func() {
			logs, err := client.GetLogs(ctx, blockHash, &contractAddress)
			Expect(err).ToNot(HaveOccurred())

			expectedLogs := []graphql.LogResponse{
				{
					Topics:      test_helpers.MockLog1.Topics,
					Data:        hexutil.Bytes(test_helpers.MockLog1.Data),
					Transaction: graphql.TransactionResp{Hash: test_helpers.MockTransactions[0].Hash()},
					ReceiptCID:  test_helpers.Rct1CID.String(),
					Status:      int32(test_helpers.MockReceipts[0].Status),
				},
			}

			Expect(logs).To(Equal(expectedLogs))
		})

		It("Retrieves logs for the failed receipt status that matches the provided blockHash and another contract address", func() {
			logs, err := client.GetLogs(ctx, blockHash, &test_helpers.AnotherAddress2)
			Expect(err).ToNot(HaveOccurred())

			expectedLogs := []graphql.LogResponse{
				{
					Topics:      test_helpers.MockLog6.Topics,
					Data:        hexutil.Bytes(test_helpers.MockLog6.Data),
					Transaction: graphql.TransactionResp{Hash: test_helpers.MockTransactions[3].Hash()},
					ReceiptCID:  test_helpers.Rct4CID.String(),
					Status:      int32(test_helpers.MockReceipts[3].Status),
				},
			}

			Expect(logs).To(Equal(expectedLogs))
		})

		It("Retrieves all the logs for the receipt that matches the provided blockHash and nil contract address", func() {
			logs, err := client.GetLogs(ctx, blockHash, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(logs)).To(Equal(6))
		})

		It("Retrieves logs with random hash", func() {
			logs, err := client.GetLogs(ctx, randomHash, &contractAddress)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(logs)).To(Equal(0))
		})
	})

	Describe("eth_getStorageAt", func() {
		It("Retrieves the storage value at the provided contract address and storage leaf key at the block with the provided hash", func() {
			storageRes, err := client.GetStorageAt(ctx, blockHashes[2], contractAddress, test_helpers.IndexOne)
			Expect(err).ToNot(HaveOccurred())
			Expect(storageRes.Value).To(Equal(common.HexToHash("01")))

			storageRes, err = client.GetStorageAt(ctx, blockHashes[3], contractAddress, test_helpers.IndexOne)
			Expect(err).ToNot(HaveOccurred())
			Expect(storageRes.Value).To(Equal(common.HexToHash("03")))

			storageRes, err = client.GetStorageAt(ctx, blockHashes[4], contractAddress, test_helpers.IndexOne)
			Expect(err).ToNot(HaveOccurred())
			Expect(storageRes.Value).To(Equal(common.HexToHash("09")))
		})

		It("Retrieves empty data if it tries to access a contract at a blockHash which does not exist", func() {
			storageRes, err := client.GetStorageAt(ctx, blockHashes[0], contractAddress, test_helpers.IndexOne)
			Expect(err).ToNot(HaveOccurred())
			Expect(storageRes.Value).To(Equal(common.Hash{}))

			storageRes, err = client.GetStorageAt(ctx, blockHashes[1], contractAddress, test_helpers.IndexOne)
			Expect(err).ToNot(HaveOccurred())
			Expect(storageRes.Value).To(Equal(common.Hash{}))
		})

		It("Retrieves empty data if it tries to access a contract slot which does not exist", func() {
			storageRes, err := client.GetStorageAt(ctx, blockHashes[3], contractAddress, randomHash.Hex())
			Expect(err).ToNot(HaveOccurred())
			Expect(storageRes.Value).To(Equal(common.Hash{}))
		})
	})
})
