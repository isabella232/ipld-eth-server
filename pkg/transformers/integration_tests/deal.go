// Copyright 2018 Vulcanize
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration_tests

import (
	"github.com/ethereum/go-ethereum/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vulcanize/vulcanizedb/pkg/core"
	"github.com/vulcanize/vulcanizedb/pkg/datastore/postgres"
	"github.com/vulcanize/vulcanizedb/pkg/transformers/deal"
	"github.com/vulcanize/vulcanizedb/pkg/transformers/factories"
	"github.com/vulcanize/vulcanizedb/pkg/transformers/shared"
	"github.com/vulcanize/vulcanizedb/pkg/transformers/shared/constants"
	"github.com/vulcanize/vulcanizedb/test_config"
)

var _ = Describe("Deal transformer", func() {
	var (
		db          *postgres.DB
		blockChain  core.BlockChain
		config      shared.TransformerConfig
		initializer factories.LogNoteTransformer
		fetcher     *shared.Fetcher
		addresses   []common.Address
		topics      []common.Hash
	)

	BeforeEach(func() {
		rpcClient, ethClient, err := getClients(ipc)
		Expect(err).NotTo(HaveOccurred())
		blockChain, err = getBlockChain(rpcClient, ethClient)
		Expect(err).NotTo(HaveOccurred())
		db = test_config.NewTestDB(blockChain.Node())
		test_config.CleanTestDB(db)

		config = deal.DealConfig

		initializer = factories.LogNoteTransformer{
			Config:     config,
			Converter:  &deal.DealConverter{},
			Repository: &deal.DealRepository{},
		}

		fetcher = shared.NewFetcher(blockChain)
		addresses = shared.HexStringsToAddresses(config.ContractAddresses)
		topics = []common.Hash{common.HexToHash(config.Topic)}

	})

	It("persists a flip deal log event", func() {
		// transaction: 0x05b5eabac2ace136f0f7e0efc61d7d42abe8e8938cc0f04fbf1a6ba545d59e58
		flipBlockNumber := int64(8958007)
		header, err := persistHeader(db, flipBlockNumber, blockChain)
		Expect(err).NotTo(HaveOccurred())

		initializer.Config.StartingBlockNumber = flipBlockNumber
		initializer.Config.EndingBlockNumber = flipBlockNumber

		logs, err := fetcher.FetchLogs(addresses, topics, header)
		Expect(err).NotTo(HaveOccurred())

		transformer := initializer.NewLogNoteTransformer(db)
		err = transformer.Execute(logs, header)
		Expect(err).NotTo(HaveOccurred())

		var dbResult []deal.DealModel
		err = db.Select(&dbResult, `SELECT bid_id, contract_address FROM maker.deal`)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(dbResult)).To(Equal(1))
		Expect(dbResult[0].BidId).To(Equal("6"))
		Expect(dbResult[0].ContractAddress).To(Equal(constants.FlipperContractAddress))
	})

	It("persists a flop deal log event", func() {
		//TODO: There are currently no Flop.deal events on Kovan
	})

	It("persists a flap deal log event", func() {
		flapBlockNumber := int64(9004628)
		header, err := persistHeader(db, flapBlockNumber, blockChain)
		Expect(err).NotTo(HaveOccurred())

		initializer.Config.StartingBlockNumber = flapBlockNumber
		initializer.Config.EndingBlockNumber = flapBlockNumber

		logs, err := fetcher.FetchLogs(addresses, topics, header)
		Expect(err).NotTo(HaveOccurred())

		transformer := initializer.NewLogNoteTransformer(db)
		err = transformer.Execute(logs, header)
		Expect(err).NotTo(HaveOccurred())

		var dbResult []deal.DealModel
		err = db.Select(&dbResult, `SELECT bid_id, contract_address FROM maker.deal`)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(dbResult)).To(Equal(1))
		Expect(dbResult[0].BidId).To(Equal("1"))
		Expect(dbResult[0].ContractAddress).To(Equal(constants.FlapperContractAddress))
	})
})