// Copyright (C) 2017 go-nebulas authors
//
// This file is part of the go-nebulas library.
//
// the go-nebulas library is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// the go-nebulas library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with the go-nebulas library.  If not, see <http://www.gnu.org/licenses/>.
//

package core

import (
	"errors"
	"fmt"
	"time"

	"encoding/json"

	"github.com/gogo/protobuf/proto"
	"github.com/nebulasio/go-nebulas/core/pb"
	"github.com/nebulasio/go-nebulas/core/state"
	"github.com/nebulasio/go-nebulas/crypto"
	"github.com/nebulasio/go-nebulas/crypto/hash"
	"github.com/nebulasio/go-nebulas/crypto/keystore"
	"github.com/nebulasio/go-nebulas/util"
	"github.com/nebulasio/go-nebulas/util/byteutils"
	"github.com/nebulasio/go-nebulas/util/logging"
	"github.com/sirupsen/logrus"
)

var (
	// TransactionMaxGasPrice max gasPrice:50 * 10 ** 9
	TransactionMaxGasPrice, _ = util.NewUint128FromString("50000000000")

	// TransactionMaxGas max gas:50 * 10 ** 9
	TransactionMaxGas, _ = util.NewUint128FromString("50000000000")

	// TransactionGasPrice default gasPrice : 10**6
	TransactionGasPrice, _ = util.NewUint128FromInt(1000000)

	// MinGasCountPerTransaction default gas for normal transaction
	MinGasCountPerTransaction, _ = util.NewUint128FromInt(20000)

	// GasCountPerByte per byte of data attached to a transaction gas cost
	GasCountPerByte, _ = util.NewUint128FromInt(1)

	// DelegateBaseGasCount is base gas count of delegate transaction
	DelegateBaseGasCount, _ = util.NewUint128FromInt(20000)

	// CandidateBaseGasCount is base gas count of candidate transaction
	CandidateBaseGasCount, _ = util.NewUint128FromInt(20000)

	// ZeroGasCount is zero gas count
	ZeroGasCount = util.NewUint128()
)

// TransactionEvent transaction event
type TransactionEvent struct {
	Hash    string `json:"hash"`
	Status  int8   `json:"status"`
	GasUsed string `json:"gas_used"`
	Error   string `json:"error"`
}

// Transaction type is used to handle all transaction data.
type Transaction struct {
	hash      byteutils.Hash
	from      *Address
	to        *Address
	value     *util.Uint128
	nonce     uint64
	timestamp int64
	data      *corepb.Data
	chainID   uint32
	gasPrice  *util.Uint128
	gasLimit  *util.Uint128

	// Signature
	alg  uint8          // algorithm, ToFix: change to keystore.Algorithm
	sign byteutils.Hash // Signature values
}

// From return from address
func (tx *Transaction) From() *Address {
	return tx.from
}

// Timestamp return timestamp
func (tx *Transaction) Timestamp() int64 {
	return tx.timestamp
}

// To return to address
func (tx *Transaction) To() *Address {
	return tx.to
}

// ChainID return chainID
func (tx *Transaction) ChainID() uint32 {
	return tx.chainID
}

// Value return tx value
func (tx *Transaction) Value() *util.Uint128 {
	return tx.value
}

// Nonce return tx nonce
func (tx *Transaction) Nonce() uint64 {
	return tx.nonce
}

// Type return tx type
func (tx *Transaction) Type() string {
	return tx.data.Type // ToFix: Check tx.data is not nil
}

// Data return tx data
func (tx *Transaction) Data() []byte {
	return tx.data.Payload // ToFix: Check tx.data is not nil
}

// ToProto converts domain Tx to proto Tx
func (tx *Transaction) ToProto() (proto.Message, error) {
	value, err := tx.value.ToFixedSizeByteSlice()
	if err != nil {
		return nil, err
	}
	gasPrice, err := tx.gasPrice.ToFixedSizeByteSlice()
	if err != nil {
		return nil, err
	}
	gasLimit, err := tx.gasLimit.ToFixedSizeByteSlice()
	if err != nil {
		return nil, err
	}
	return &corepb.Transaction{
		Hash:      tx.hash,
		From:      tx.from.address,
		To:        tx.to.address,
		Value:     value,
		Nonce:     tx.nonce,
		Timestamp: tx.timestamp,
		Data:      tx.data,
		ChainId:   tx.chainID,
		GasPrice:  gasPrice,
		GasLimit:  gasLimit,
		Alg:       uint32(tx.alg),
		Sign:      tx.sign,
	}, nil
}

// FromProto converts proto Tx into domain Tx
func (tx *Transaction) FromProto(msg proto.Message) error { // ToFix: check msg is not nil.
	if msg, ok := msg.(*corepb.Transaction); ok {
		tx.hash = msg.Hash
		tx.from = &Address{msg.From} // ToFix: Check Address, use AddressParse
		tx.to = &Address{msg.To}
		value, err := util.NewUint128FromFixedSizeByteSlice(msg.Value)
		if err != nil {
			return err
		}
		tx.value = value // ToFix: Check value is not nil
		tx.nonce = msg.Nonce
		tx.timestamp = msg.Timestamp
		tx.data = msg.Data // ToFix: Check msg.data is not nil // ToCheck: length <= 1m
		tx.chainID = msg.ChainId
		gasPrice, err := util.NewUint128FromFixedSizeByteSlice(msg.GasPrice)
		if err != nil {
			return err
		}
		tx.gasPrice = gasPrice
		gasLimit, err := util.NewUint128FromFixedSizeByteSlice(msg.GasLimit)
		if err != nil {
			return err
		}
		tx.gasLimit = gasLimit
		tx.alg = uint8(msg.Alg)
		tx.sign = msg.Sign
		return nil
	}
	return errors.New("Protobuf Message cannot be converted into Transaction")
}

func (tx *Transaction) String() string {
	return fmt.Sprintf(`{"chainID":%d, "hash":"%s", "from":"%s", "to":"%s", "nonce":%d, "value":"%s", "timestamp":%d, "gasprice": "%s", "gaslimit":"%s", "type":"%s"}`,
		tx.chainID,
		tx.hash.String(),
		tx.from.String(),
		tx.to.String(),
		tx.nonce,
		tx.value.String(),
		tx.timestamp,
		tx.gasPrice.String(),
		tx.gasLimit.String(),
		tx.Type(),
	) // ToFix: Check Hash is not nil
}

// Transactions is an alias of Transaction array.
type Transactions []*Transaction

// NewTransaction create #Transaction instance.
func NewTransaction(chainID uint32, from, to *Address, value *util.Uint128, nonce uint64, payloadType string, payload []byte, gasPrice *util.Uint128, gasLimit *util.Uint128) *Transaction { // ToFix: check args
	//if gasPrice is not specified, use the default gasPrice
	if gasPrice == nil || gasPrice.Cmp(util.NewUint128()) <= 0 { // ToCheck: default value is reasonable?
		gasPrice = TransactionGasPrice // ToConfirm: uint128 should be immutable
	}
	if gasLimit == nil || gasLimit.Cmp(util.NewUint128()) <= 0 {
		gasLimit = MinGasCountPerTransaction
	}

	tx := &Transaction{
		from:      from,  // ToFix:  check nil
		to:        to,    // ToFix: check nil
		value:     value, // ToFix: check nil
		nonce:     nonce,
		timestamp: time.Now().Unix(),
		chainID:   chainID,
		data:      &corepb.Data{Type: payloadType, Payload: payload}, // ToCheck: length <= 1m
		gasPrice:  gasPrice,                                          // ToFix: check nil
		gasLimit:  gasLimit,                                          // ToFix: check nil
	}
	return tx
}

// Hash return the hash of transaction.
func (tx *Transaction) Hash() byteutils.Hash {
	return tx.hash
}

// GasPrice returns gasPrice
func (tx *Transaction) GasPrice() *util.Uint128 {
	return tx.gasPrice
}

// GasLimit returns gasLimit
func (tx *Transaction) GasLimit() *util.Uint128 {
	return tx.gasLimit
}

// PayloadGasLimit returns payload gasLimit
func (tx *Transaction) PayloadGasLimit(payload TxPayload) (*util.Uint128, error) { // ToFix: check args.
	// payloadGasLimit = tx.gasLimit - tx.GasCountOfTxBase
	gasCountOfTxBase, err := tx.GasCountOfTxBase()
	if err != nil {
		return nil, err
	}
	payloadGasLimit, err := tx.gasLimit.Sub(gasCountOfTxBase)
	if err != nil {
		return nil, ErrOutOfGasLimit
	}
	payloadGasLimit, err = payloadGasLimit.Sub(payload.BaseGasCount())
	if err != nil {
		return nil, ErrOutOfGasLimit
	}
	return payloadGasLimit, nil
}

// MinBalanceRequired returns gasprice * gaslimit.
func (tx *Transaction) MinBalanceRequired() (*util.Uint128, error) {
	total, err := tx.GasPrice().Mul(tx.GasLimit()) // ToConfirm: balance >= gaslimit * gasprice + value
	if err != nil {
		return nil, err
	}
	return total, nil
}

// GasCountOfTxBase calculate the actual amount for a tx with data
func (tx *Transaction) GasCountOfTxBase() (*util.Uint128, error) {
	txGas := MinGasCountPerTransaction.DeepCopy() // ToConfirm: DeepCopy nessasary?
	if tx.DataLen() > 0 {
		dataLen, err := util.NewUint128FromInt(int64(tx.DataLen()))
		if err != nil {
			return nil, err
		}
		dataGas, err := dataLen.Mul(GasCountPerByte)
		if err != nil {
			return nil, err
		}
		txGas, err = txGas.Add(dataGas)
		if err != nil {
			return nil, err
		}
	}
	return txGas, nil
}

// DataLen return the length of payload
func (tx *Transaction) DataLen() int {
	return len(tx.data.Payload) // ToCheck: missing type? // ToFix: check tx.data is nil
}

// LoadPayload returns tx's payload
func (tx *Transaction) LoadPayload(block *Block) (TxPayload, error) { // ToFix: check args.
	// execute payload
	var (
		payload TxPayload
		err     error
	)
	switch tx.data.Type {
	case TxPayloadBinaryType:
		if block.height > OptimizeHeight {
			payload, err = LoadBinaryPayload(tx.data.Payload)
		} else {
			if block.Height() >= 280921 && block.Height() <= 297680 || block.Height() >= 300087 && block.Height() <= 302302 {
				payload, err = LoadBinaryPayloadDeprecatedFail(tx.data.Payload)
			} else {
				payload, err = LoadBinaryPayloadDeprecated(tx.data.Payload)
			}
		}
	case TxPayloadDeployType:
		payload, err = LoadDeployPayload(tx.data.Payload)
	case TxPayloadCallType:
		payload, err = LoadCallPayload(tx.data.Payload)
	case TxPayloadCandidateType: // ToConfirm: Delete
		payload, err = LoadCandidatePayload(tx.data.Payload)
	case TxPayloadDelegateType: // ToConfirm: Delete
		payload, err = LoadDelegatePayload(tx.data.Payload)
	default:
		err = ErrInvalidTxPayloadType
	}
	return payload, err
}

// LocalExecution returns tx local execution
func (tx *Transaction) LocalExecution(block *Block) (*util.Uint128, string, error) { // ToFix: check args.
	hash, err := HashTransaction(tx) // ToFix: tx should have hash here
	if err != nil {
		return nil, "", err
	}
	tx.hash = hash

	txBlock, err := block.Clone()
	if err != nil {
		return nil, "", err
	}

	txBlock.begin()
	defer txBlock.rollback()

	payload, err := tx.LoadPayload(txBlock)
	if err != nil {
		return util.NewUint128(), "", err
	}

	gasUsed, err := tx.GasCountOfTxBase()
	if err != nil {
		return util.NewUint128(), "", err
	}
	gasUsed, err = gasUsed.Add(payload.BaseGasCount())
	if err != nil {
		return util.NewUint128(), "", err
	}

	gasExecution, result, exeErr := payload.Execute(txBlock, tx)

	gas, err := gasUsed.Add(gasExecution)
	if err != nil {
		return gasUsed, result, err
	}
	return gas, result, exeErr
}

// VerifyExecution transaction and return result.
func (tx *Transaction) VerifyExecution(block *Block) (*util.Uint128, error) { // ToRefine: multiple version for compatible codes. divide into smaller pieces. check args.
	// check balance.
	fromAcc, err := block.accState.GetOrCreateUserAccount(tx.from.address)
	if err != nil {
		return nil, err
	}
	toAcc, err := block.accState.GetOrCreateUserAccount(tx.to.address)
	if err != nil {
		return nil, err
	}
	coinbaseAcc, err := block.accState.GetOrCreateUserAccount(block.CoinbaseHash())
	if err != nil {
		return nil, err
	}

	// balance < gasLimit*gasPric
	minBalanceRequired, err := tx.MinBalanceRequired() // ToConfirm: Check balance earlier
	if err != nil {
		return nil, err
	}
	if fromAcc.Balance().Cmp(minBalanceRequired) < 0 {
		return util.NewUint128(), ErrInsufficientBalance
	}

	//TODO: later remove TransactionOptimizeHeight
	if block.height > TransactionOptimizeHeight { // ToAdd: compatiable codes comment
		minBalanceRequired, err = minBalanceRequired.Add(tx.value)
		if err != nil {
			return nil, err
		}
		if fromAcc.Balance().Cmp(minBalanceRequired) < 0 {
			return nil, ErrInsufficientBalance
		}
	}

	// gasLimit < gasUsed
	gasUsed, err := tx.GasCountOfTxBase()
	if err != nil {
		return nil, err
	}
	if tx.gasLimit.Cmp(gasUsed) < 0 {
		logging.VLog().WithFields(logrus.Fields{
			"error":       ErrOutOfGasLimit,
			"transaction": tx,
			"limit":       tx.gasLimit.String(),
			"used":        gasUsed.String(),
		}).Debug("Failed to store the payload on chain.")
		return util.NewUint128(), ErrOutOfGasLimit
	}

	payload, err := tx.LoadPayload(block)
	if err != nil {
		logging.VLog().WithFields(logrus.Fields{
			"error":       err,
			"block":       block,
			"transaction": tx,
		}).Debug("Failed to load payload.")
		metricsTxExeFailed.Mark(1)

		tmpErr := tx.gasConsumption(fromAcc, coinbaseAcc, gasUsed)
		if tmpErr != nil {
			return nil, tmpErr
		}
		tx.triggerEvent(TopicExecuteTxFailed, block, gasUsed, err)
		return gasUsed, nil
	}

	gasUsed, err = gasUsed.Add(payload.BaseGasCount())
	if err != nil {
		return nil, err
	}
	if tx.gasLimit.Cmp(gasUsed) < 0 {
		logging.VLog().WithFields(logrus.Fields{
			"err":   ErrOutOfGasLimit,
			"block": block,
			"tx":    tx,
		}).Debug("Failed to check base gas used.")
		metricsTxExeFailed.Mark(1)

		tmpErr := tx.gasConsumption(fromAcc, coinbaseAcc, tx.gasLimit)
		if tmpErr != nil {
			return nil, tmpErr
		}
		tx.triggerEvent(TopicExecuteTxFailed, block, tx.gasLimit, ErrOutOfGasLimit)
		return tx.gasLimit, nil
	}

	// block begin
	txBlock, err := block.Clone()
	if err != nil {
		return util.NewUint128(), err
	}

	// execute smart contract and sub the calcute gas.
	gasExecution, _, exeErr := payload.Execute(txBlock, tx)

	// gas = tx.GasCountOfTxBase() +  gasExecution
	gas, gasErr := gasUsed.Add(gasExecution)
	if gasErr != nil {
		return nil, gasErr
	}

	//TODO: later remove TransactionOptimizeHeight
	if block.height > TransactionOptimizeHeight { // ToAdd: compatible codes comment
		if tx.gasLimit.Cmp(gas) < 0 {
			gas = tx.gasLimit
			exeErr = ErrOutOfGasLimit
		}
	}

	// only execute success, merge the state to use
	if exeErr == nil {
		block.Merge(txBlock)
	}

	fromAcc, err = block.accState.GetOrCreateUserAccount(tx.from.address)
	if err != nil {
		return nil, err
	}
	toAcc, err = block.accState.GetOrCreateUserAccount(tx.to.address)
	if err != nil {
		return nil, err
	}
	coinbaseAcc, err = block.accState.GetOrCreateUserAccount(block.CoinbaseHash())
	if err != nil {
		return nil, err
	}

	gasErr = tx.gasConsumption(fromAcc, coinbaseAcc, gas)
	if gasErr != nil {
		return nil, gasErr
	}
	if exeErr != nil {
		logging.VLog().WithFields(logrus.Fields{
			"exeErr":       exeErr,
			"block":        block,
			"tx":           tx,
			"gasUsed":      gasUsed.String(),
			"gasExecution": gasExecution.String(),
		}).Debug("Failed to execute payload.")

		metricsTxExeFailed.Mark(1)

		// TODO: later remove. Compatible with older version error data.
		if block.height < TransactionOptimizeHeight { // ToAdd: compatiable codes comment.
			exeErr = err // ToFix: change to nil, easy to understand
		}

		tx.triggerEvent(TopicExecuteTxFailed, block, gas, exeErr)
	} else {
		if fromAcc.Balance().Cmp(tx.value) < 0 {
			logging.VLog().WithFields(logrus.Fields{
				"exeErr": ErrInsufficientBalance,
				"block":  block,
				"tx":     tx,
			}).Debug("Failed to check balance sufficient.")

			metricsTxExeFailed.Mark(1)
			tx.triggerEvent(TopicExecuteTxFailed, block, gas, ErrInsufficientBalance)
		} else {
			// accept the transaction
			fromAcc.SubBalance(tx.value)
			err := toAcc.AddBalance(tx.value)
			if err != nil {
				return nil, err
			}
			metricsTxExeSuccess.Mark(1)
			// record tx execution success event
			tx.triggerEvent(TopicExecuteTxSuccess, block, gas, nil)
		}
	}

	return gas, nil
}

func (tx *Transaction) gasConsumption(from, coinbase state.Account, gas *util.Uint128) error { // ToFix: diff gas & gasCnt.
	gasCost, err := tx.GasPrice().Mul(gas)
	if err != nil {
		return err
	}
	err = from.SubBalance(gasCost)
	if err != nil {
		return err
	}
	err = coinbase.AddBalance(gasCost)
	return err
}

func (tx *Transaction) triggerEvent(topic string, block *Block, gasUsed *util.Uint128, err error) { // ToRefine: better err name

	// Notice: We updated the definition of the transaction result event,
	// and the event is recorded on the chain, so it needs to be compatible.
	if block.Height() > OptimizeHeight { // ToCheck: check block is not nil.
		tx.recordResultEvent(block, gasUsed, err)
		return
	}

	// deprecated for new block mined
	var txData []byte
	pbTx, _ := tx.ToProto() // ToFix: catch err
	if err != nil {
		var (
			txErrEvent struct {
				Transaction proto.Message `json:"transaction"`
				Error       error         `json:"error"`
			}
		)
		txErrEvent.Transaction = pbTx
		txErrEvent.Error = err
		txData, _ = json.Marshal(txErrEvent) // ToFix: catch err
	} else {
		txData, _ = json.Marshal(pbTx) // ToFix: catch err
	}

	event := &Event{Topic: topic,
		Data: string(txData)}
	block.recordEvent(tx.hash, event)
}

func (tx *Transaction) recordResultEvent(block *Block, gasUsed *util.Uint128, err error) {

	txEvent := &TransactionEvent{
		Hash:    tx.hash.String(),
		GasUsed: gasUsed.String(),
	}
	if err != nil {
		txEvent.Status = TxExecutionFailed
		txEvent.Error = err.Error()
	} else {
		txEvent.Status = TxExecutionSuccess
	}

	txData, _ := json.Marshal(txEvent) // ToFix: catch err
	//logging.VLog().WithFields(logrus.Fields{
	//	"topic": TopicTransactionExecutionResult,
	//	"event": string(txData),
	//}).Debug("record event.")
	event := &Event{Topic: TopicTransactionExecutionResult,
		Data: string(txData)}
	block.recordEvent(tx.hash, event)
}

// Sign sign transaction,sign algorithm is
func (tx *Transaction) Sign(signature keystore.Signature) error { // ToCheck: signature is not nil.
	hash, err := HashTransaction(tx)
	if err != nil {
		return err
	}
	sign, err := signature.Sign(hash)
	if err != nil {
		return err
	}
	tx.hash = hash
	tx.alg = uint8(signature.Algorithm())
	tx.sign = sign
	return nil
}

// VerifyIntegrity return transaction verify result, including Hash and Signature.
func (tx *Transaction) VerifyIntegrity(chainID uint32) error {
	// check ChainID.
	if tx.chainID != chainID {
		return ErrInvalidChainID
	}

	// check Hash.
	wantedHash, err := HashTransaction(tx)
	if err != nil {
		return err
	}
	if wantedHash.Equals(tx.hash) == false {
		return ErrInvalidTransactionHash
	}

	// check Signature.
	return tx.verifySign()

}

func (tx *Transaction) verifySign() error {
	signature, err := crypto.NewSignature(keystore.Algorithm(tx.alg))
	if err != nil {
		return err
	}
	pub, err := signature.RecoverPublic(tx.hash, tx.sign)
	if err != nil {
		return err
	}
	pubdata, err := pub.Encoded()
	if err != nil {
		return err
	}
	addr, err := NewAddressFromPublicKey(pubdata)
	if err != nil {
		return err
	}
	if !tx.from.Equals(addr) {
		logging.VLog().WithFields(logrus.Fields{
			"recover address": addr.String(),
			"tx":              tx,
		}).Debug("Failed to verify tx's sign.")
		return ErrInvalidTransactionSigner
	}
	return nil
}

// GenerateContractAddress according to tx.from and tx.nonce.
func (tx *Transaction) GenerateContractAddress() (*Address, error) {
	return NewContractAddressFromHash(hash.Sha3256(tx.from.Bytes(), byteutils.FromUint64(tx.nonce)))
}

// HashTransaction hash the transaction.
func HashTransaction(tx *Transaction) (byteutils.Hash, error) {
	value, err := tx.value.ToFixedSizeByteSlice()
	if err != nil {
		return nil, err
	}
	data, err := proto.Marshal(tx.data)
	if err != nil {
		return nil, err
	}
	gasPrice, err := tx.gasPrice.ToFixedSizeByteSlice()
	if err != nil {
		return nil, err
	}
	gasLimit, err := tx.gasLimit.ToFixedSizeByteSlice()
	if err != nil {
		return nil, err
	}
	return hash.Sha3256(
		tx.from.address,
		tx.to.address,
		value,
		byteutils.FromUint64(tx.nonce),
		byteutils.FromInt64(tx.timestamp),
		data,
		byteutils.FromUint32(tx.chainID),
		gasPrice,
		gasLimit,
	), nil
}
