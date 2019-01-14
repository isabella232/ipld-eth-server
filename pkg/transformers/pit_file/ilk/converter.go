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

package ilk

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/vulcanize/vulcanizedb/pkg/transformers/shared"
	"github.com/vulcanize/vulcanizedb/pkg/transformers/shared/constants"
)

type PitFileIlkConverter struct{}

func (PitFileIlkConverter) ToModels(ethLogs []types.Log) ([]interface{}, error) {
	var models []interface{}
	for _, ethLog := range ethLogs {
		err := verifyLog(ethLog)
		if err != nil {
			return nil, err
		}
		ilk := string(bytes.Trim(ethLog.Topics[2].Bytes(), "\x00"))
		what := string(bytes.Trim(ethLog.Topics[3].Bytes(), "\x00"))
		dataBytes := ethLog.Data[len(ethLog.Data)-constants.DataItemLength:]
		data, err := getData(dataBytes, what)
		if err != nil {
			return nil, err
		}

		raw, err := json.Marshal(ethLog)
		if err != nil {
			return nil, err
		}
		model := PitFileIlkModel{
			Ilk:              ilk,
			What:             what,
			Data:             data,
			LogIndex:         ethLog.Index,
			TransactionIndex: ethLog.TxIndex,
			Raw:              raw,
		}
		models = append(models, model)
	}
	return models, nil
}

func getData(dataBytes []byte, what string) (string, error) {
	n := big.NewInt(0).SetBytes(dataBytes).String()
	if what == "spot" {
		return shared.ConvertToRay(n), nil
	} else if what == "line" {
		return shared.ConvertToWad(n), nil
	} else {
		return "", errors.New("unexpected payload for 'what'")
	}
}

func verifyLog(log types.Log) error {
	if len(log.Topics) < 4 {
		return errors.New("log missing topics")
	}
	if len(log.Data) < constants.DataItemLength {
		return errors.New("log missing data")
	}
	return nil
}