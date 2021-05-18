package lib

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/shibukawa/configdir"
)

const (
	// ConfigDirVendorName is the enclosing folder for user data.
	// It's required to created a ConfigDir.
	ConfigDirVendorName = "bitclout"
	// ConfigDirAppName is the folder where we keep user data.
	ConfigDirAppName = "bitclout"
	// UseridLengthBytes is the number of bytes of entropy to use for
	// a userid.
	UseridLengthBytes = 32

	// These constants are used by the DNS seed code to pick a random last
	// seen time.
	SecondsIn3Days int32 = 24 * 60 * 60 * 3
	SecondsIn4Days int32 = 24 * 60 * 60 * 4

	// MessagesToFetchPerCall is used to limit the number of messages to fetch
	// when getting a user's inbox.
	MessagesToFetchPerInboxCall = 10000
)

type NetworkType uint64

const (
	// The different network types. For now we have a mainnet and a testnet.
	// Also create an UNSET value to catch errors.
	NetworkType_UNSET   NetworkType = 0
	NetworkType_MAINNET NetworkType = 1
	NetworkType_TESTNET NetworkType = 2
)

const (
	// This is the header version that the blockchain started with.
	HeaderVersion0 = uint32(0)
	// This version made several changes to the previous header encoding format:
	// - The Nonce field was expanded to 64 bits
	// - Another ExtraNonce field was added to provide *another* 64 bits of entropy,
	//   for a total of 128 bits of entropy in the header that miners can twiddle.
	// - The header height was expanded to 64 bits
	// - The TstampSecs were expanded to 64 bits
	// - All fields were moved from encoding in little-endian to big-endian
	//
	// The benefit of this change is that miners can hash over a wider space without
	// needing to twiddle ExtraData ever.
	//
	// At the time of this writing, the intent is to deploy it in a backwards-compatible
	// fashion, with the eventual goal of phasing out blocks with the previous version.
	HeaderVersion1       = uint32(1)
	CurrentHeaderVersion = HeaderVersion1
)

var (
	// The block height at which various forks occurred including an
	// explanation as to why they're necessary.

	// SalomonFixBlockHeight defines a block height where the protocol implements
	// two changes:
	// 	(1) The protocol now prints founder reward for all buy transactions instead
	//		of just when creators reach a new all time high.
	//		This was decided in order to provide lasting incentive for creators
	//		to utilize the protocol.
	//	(2) A fix was created to deal with a bug accidentally triggered by @salomon.
	//		After a series of buys and sells @salomon was left with a single creator coin
	//		nano in circulation and a single BitClout nano locked. This caused a detach
	//		between @salomon's bonding curve and others on the protocol. As more buys and sells
	//		continued, @salomon's bonding curve continued to detach further and further from its peers.
	// 		At its core, @salomon had too few creator coins in circulation. This fix introduces
	//		this missing supply back into circulation as well as prevented detached Bancor bonding
	//		curves from coming into existence.
	//		^ It was later decided to leave Salomon's coin circulation alone. A fix was introduced
	//		to prevent similar cases from occurring again, but @salomon is left alone.
	SalomonFixBlockHeight = uint32(15270)

	// BitCloutFounderRewardBlockHeight defines a block height where the protocol switches from
	// paying the founder reward in the founder's own creator coin to paying in BitClout instead.
	BitCloutFounderRewardBlockHeight = uint32(21869)
)

func (nt NetworkType) String() string {
	switch nt {
	case NetworkType_UNSET:
		return "UNSET"
	case NetworkType_MAINNET:
		return "MAINNET"
	case NetworkType_TESTNET:
		return "TESTNET"
	default:
		return fmt.Sprintf("UNRECOGNIZED(%d) - make sure String() is up to date", nt)
	}
}

const (
	MaxUsernameLengthBytes = 25
)

var (
	UsernameRegex = regexp.MustCompile("^[a-zA-Z0-9_]+$")
	// Profile pics are Base64 encoded plus ": ; ," used in the mime type spec.
	ProfilePicRegex = regexp.MustCompile("^[a-zA-Z0-9+/:;,]+$")

	TikTokShortURLRegex = regexp.MustCompile("^.*(vm\\.tiktok\\.com/)([A-Za-z0-9]{6,12}).*")
	TikTokFullURLRegex  = regexp.MustCompile("^.*((tiktok\\.com/)(v/)|(@[A-Za-z0-9_-]{2,24}/video/)|(embed/v2/))(\\d{0,30}).*")
)

// BitCloutParams defines the full list of possible parameters for the
// BitClout network.
type BitCloutParams struct {
	// The network type (mainnet, testnet, etc).
	NetworkType NetworkType
	// The current protocol version we're running.
	ProtocolVersion uint64
	// The minimum protocol version we'll allow a peer we connect to
	// to have.
	MinProtocolVersion uint64
	// Used as a "vanity plate" to identify different BitClout
	// clients. Mainly useful in analyzing the network at
	// a meta level, not in the protocol itself.
	UserAgent string
	// The list of DNS seed hosts to use during bootstrapping.
	DNSSeeds []string

	// A list of DNS seed prefixes and suffixes to use during bootstrapping.
	// These prefixes and suffixes will be scanned and all IPs found will be
	// incorporated into the address manager.
	DNSSeedGenerators [][]string

	// BitcoinDNSSeeds is a list of seed hosts to use to bootstrap connections
	// to Bitcoin nodes. We connect to Bitcoin nodes to support the exchange
	// functionality that allows people to buy BitClout with Bitcoin.
	BitcoinDNSSeeds []string

	// The minimum amount of work a Bitcoin chain can have before we consider
	// it valid.
	BitcoinMinChainWorkHex string

	// The default port to connect to bitcoin nodes on.
	BitcoinDefaultPort string

	// The network parameter for Bitcoin messages as defined by the btcd library.
	// Useful for certain function calls we make to this library.
	BitcoinBtcdParams *chaincfg.Params

	// The version of the Bitcoin protocol we use.
	BitcoinProtocolVersion uint32

	BitcoinBlocksPerRetarget uint32

	BitcoinPowLimitBits uint32

	// Testnet only. Ignored if set to zero.
	BitcoinMinDiffReductionTime time.Duration

	BitcoinTargetTimespanSecs      uint32
	BitcoinMinRetargetTimespanSecs uint32
	BitcoinMaxRetargetTimespanSecs uint32

	// The maximum number of seconds in a future a Bitcoin block timestamp is allowed
	// to be before it is rejected.
	BitcoinMaxTstampOffsetSeconds uint64

	// This value is used to determine whether or not the Bitcoin tip is up-to-date.
	// If the Bitcoin tip's timestamp is greater than this value then a node should
	// assume that it needs to download more Bitcoin headers from its peers before it
	// is current.
	BitcoinMaxTipAge time.Duration

	// The time between Bitcoin blocks (=10m normally)
	BitcoinTimeBetweenBlocks time.Duration

	// When someone wants to convert BitClout to Bitcoin, they send Bitcoin to the burn address
	// and then create a transaction on the BitClout chain referencing the burn from the
	// Bitcoin chain. This variable is the number of blocks that must be mined on top
	// of the initial Bitcoin burn before the corresponding BitClout transaction can be
	// validated on the BitClout chain.
	//
	// Note that, in order to make validation consistent across all nodes, even nodes
	// whose Bitcoin tip may be slightly behind, we define a very specific way of
	// computing how much work has been done for a given Bitcoin block. We define this
	// as follows:
	// - Compute the first Bitcoin block that has a timestamp less than MaxTipAge.
	//   Call this block the StalestBitcoinBlock.
	//   * Note that any node that believes itself to be up-to-date will be able to
	//     define a StalestBitcoinBlock that should be consistent with all other nodes
	//     that have a roughly similar timestamp (+/- one block in edge cases due to
	//     the timestamp being off).
	// - Suppose we have a Bitcoin burn transaction that was mined into a block. Define
	//   the work that has been done on this block as:
	//   * BitcoinWork = (StalestBitcoinBlock.Height - BitcoinBurnTransactionBlockHeight)
	//
	// Defining the BitcoinWork in this way ensures that the following holds:
	// - For a given Bitcoin transaction, if a particular node believes its Bitcoin tip
	//   to be up-to-date, then the BitcoinWork it computes for that transaction will be
	//   consistent with all other nodes that believe their Bitcoin tip to be up-to-date
	//   (+/- one block due to the timestamps being slightly off).
	// - Note that if a particular node does *not* believe its
	//   Bitcoin tip to be up-to-date, it will not process any Bitcoin burn transactions
	//   and so it does not matter that it cannot compute a value for BitcoinWork.
	//
	// Note that if we did not define BitcoinWork the way we do above and instead
	// defined it relative to a particular node's Bitcoin tip, for example, then
	// nodes could have vastly different values for BitcoinWork for the same transaction,
	// which would cause them to disagree about which Bitcoin burn transactions
	// are valid and which aren't.
	//
	// Given the above definition for BitcoinWork, miners can assure with near-certainty
	// that a block containing a Bitcoin burn transaction will be accepted by 100% of nodes
	// that are up-to-date as long as they ensure that all of the Bitcoin burn transactions
	// that they mine into their block have (BitcoinWork >= BitcoinMinBurnWorkBlocks + 1). Note that
	// nodes whose Bitcoin tip is not up-to-date will not process blocks until their tip
	// is up-to-date, which means they will also eventually accept this block as well (and
	// will not reject the block before they are up-to-date). Note also that using
	// BitcoinMinBurnWorkBlocks+1 rather than BitcoinMinBurnWorkBlocks is a good idea because it protects
	// against situations where nodes have slightly out-of-sync timestamps. In particular,
	// any node whose timestamp is between:
	// - (minerTstamp - minTimeBetweenBlocks) <= nodeTimestamp <= infinity
	// - where:
	//   * minTimeBetweenBlocks = (
	//       StalestBitcoinBlock.TImestamp - BlockRightBeforeStalestBitcoinBlock.Timestamp)
	//   * Note minTimeBetweenBlocks will be ~10m on average.
	// Will believe an BitClout block containing a Bitcoin burn transaction to be valid if the
	// Miner computes that
	// - BitcoinWork >= BitcoinMinBurnWorkBlocks+1
	//
	// If the miner wants to ride the edge, she can mine transactions when
	// BitcoinWork >= BitcoinMinBurnWorkBlocks (without hte +1 buffer). However, in this case
	// nodes who have timesatamps within:
	// - nodeTstamp <= minerTstamp - minTimeBetweenBlocks
	// - where minTImeBetweenBlocks is defined as above
	// will initially reject any BitClout blocks that contain such Bitcoin burn transactions,
	// which puts the block at risk of not being the main chain, particularly if other
	// miners prefer not to build on such blocks due to the risk of rejection.
	BitcoinMinBurnWorkBlockss      int64
	MinerBitcoinMinBurnWorkBlockss int64

	// Because we use the Bitcoin header chain only to process exchanges from
	// BTC to BitClout, we don't need to worry about Bitcoin blocks before a certain
	// point, which is specified by this node. This is basically used to make
	// header download more efficient but it's important to note that if for
	// some reason there becomes a different main chain that is stronger than
	// this one, then we will still switch to that one even with this parameter
	// set such as it is.
	BitcoinStartBlockNode *BlockNode

	// The base58Check-encoded Bitcoin address that users must send Bitcoin to in order
	// to purchase BitClout. Note that, unfortunately, simply using an all-zeros or
	// mostly-all-zeros address or public key doesn't work and, in fact, I found that
	// using almost any address other than this one also doesn't work.
	BitcoinBurnAddress string

	// This is a fee in basis points charged on BitcoinExchange transactions that gets
	// paid to the miners. Basically, if a user burned enough Satoshi to create 100 BitClout,
	// and if the BitcoinExchangeFeeBasisPoints was 1%, then 99 BitClout would be allocated to
	// the user's public key while 1 BitClout would be left as a transaction fee to the miner.
	BitcoinExchangeFeeBasisPoints uint64

	// The amount of time to wait for a Bitcoin txn to broadcast throughout the Bitcoin
	// network before checking for double-spends.
	BitcoinDoubleSpendWaitSeconds float64

	// This field allows us to set the amount purchased at genesis to a non-zero
	// value.
	BitCloutNanosPurchasedAtGenesis uint64

	// Port used for network communications among full nodes.
	DefaultSocketPort uint16
	// Port used for the limited JSON API that supports light clients.
	DefaultJSONPort uint16

	// The amount of time we wait when connecting to a peer.
	DialTimeout time.Duration
	// The amount of time we wait to receive a version message from a peer.
	VersionNegotiationTimeout time.Duration

	// The genesis block to use as the base of our chain.
	GenesisBlock *MsgBitCloutBlock
	// The expected hash of the genesis block. Should align with what one
	// would get from actually hashing the provided genesis block.
	GenesisBlockHashHex string
	// How often we target a single block to be generated.
	TimeBetweenBlocks time.Duration
	// How many blocks between difficulty retargets.
	TimeBetweenDifficultyRetargets time.Duration
	// Block hashes, when interpreted as big-endian big integers, must be
	// values less than or equal to the difficulty
	// target. The difficulty target is expressed below as a big-endian
	// big integer and is adjusted every TargetTimePerBlock
	// order to keep blocks generating at consistent intervals.
	MinDifficultyTargetHex string
	// We will reject chains that have less than this amount of total work,
	// expressed as a hexadecimal big-endian bigint. Useful for preventing
	// disk-fill attacks, among other things.
	MinChainWorkHex string

	// This is used for determining whether we are still in initial block download.
	// If our tip is older than this, we continue with IBD.
	MaxTipAge time.Duration

	// Do not allow the difficulty to change by more than a factor of this
	// variable during each adjustment period.
	MaxDifficultyRetargetFactor int64
	// Amount of time one must wait before a block reward can be spent.
	BlockRewardMaturity time.Duration
	// When shifting from v0 blocks to v1 blocks, we changed the hash function to
	// CloutHash, which is technically easier. Thus we needed to apply an adjustment
	// factor in order to phase it in.
	V1DifficultyAdjustmentFactor int64

	// The maximum number of seconds in a future a block timestamp is allowed
	// to be before it is rejected.
	MaxTstampOffsetSeconds uint64

	// The maximum number of bytes that can be allocated to transactions in
	// a block.
	MaxBlockSizeBytes uint64

	// It's useful to set the miner maximum block size to a little lower than the
	// maximum block size in certain cases. For example, on initial launch, setting
	// it significantly lower is a good way to avoid getting hit by spam blocks.
	MinerMaxBlockSizeBytes uint64

	// In order to make public keys more human-readable, we convert
	// them to base58. When we do that, we use a prefix that makes
	// the public keys to become more identifiable. For example, all
	// mainnet public keys start with "X" because we do this.
	Base58PrefixPublicKey  [3]byte
	Base58PrefixPrivateKey [3]byte

	// MaxFetchBlocks is the maximum number of blocks that can be fetched from
	// a peer at one time.
	MaxFetchBlocks uint32

	MiningIterationsPerCycle uint64

	// bitclout
	MaxUsernameLengthBytes        uint64
	MaxUserDescriptionLengthBytes uint64
	MaxProfilePicLengthBytes      uint64
	MaxProfilePicDimensions       uint64
	MaxPrivateMessageLengthBytes  uint64

	StakeFeeBasisPoints         uint64
	MaxPostBodyLengthBytes      uint64
	MaxPostSubLengthBytes       uint64
	MaxStakeMultipleBasisPoints uint64
	MaxCreatorBasisPoints       uint64
	ParamUpdaterPublicKeys      map[PkMapKey]bool

	// A list of transactions to apply when initializing the chain. Useful in
	// cases where we want to hard fork or reboot the chain with specific
	// transactions applied.
	SeedTxns []string

	// A list of balances to initialize the blockchain with. This is useful for
	// testing and useful in the event that the devs need to hard fork the chain.
	SeedBalances []*BitCloutOutput

	// This is a small fee charged on creator coin transactions. It helps
	// prevent issues related to floating point calculations.
	CreatorCoinTradeFeeBasisPoints uint64
	// These two params define the "curve" that we use when someone buys/sells
	// creator coins. Effectively, this curve amounts to a polynomial of the form:
	// - currentCreatorCoinPrice ~= slope * currentCreatorCoinSupply^(1/reserveRatio-1)
	// Buys and sells effectively take the integral of the curve in opposite directions.
	//
	// To better understand where this curve comes from and how it works, check out
	// the following links. They are all well written so don't be intimidated/afraid to
	// dig in and read them:
	// - Primer on bonding curves: https://medium.com/@simondlr/tokens-2-0-curved-token-bonding-in-curation-markets-1764a2e0bee5
	// - The Uniswap v2 white paper: https://whitepaper.io/document/600/uniswap-whitepaper
	// - The Bancor white paper: https://whitepaper.io/document/52/bancor-whitepaper
	// - Article relating Bancor curves to polynomial curves: https://medium.com/@aventus/token-bonding-curves-547f3a04914
	// - Derivations of the Bancor supply increase/decrease formulas: https://blog.relevant.community/bonding-curves-in-depth-intuition-parametrization-d3905a681e0a
	// - Implementations of Bancor equations in Solidity with code: https://yos.io/2018/11/10/bonding-curves/
	// - Bancor is flawed blog post discussing Bancor edge cases: https://hackingdistributed.com/2017/06/19/bancor-is-flawed/
	// - A mathematica equation sheet with tests that walks through all the
	//   equations. You will need to copy this into a Mathematica notebook to
	//   run it: https://pastebin.com/raw/M4a1femY
	CreatorCoinSlope        *big.Float
	CreatorCoinReserveRatio *big.Float

	// CreatorCoinAutoSellThresholdNanos defines two things. The first is the minimum amount
	// of creator coins a user must purchase in order for a transaction to be valid. Secondly
	// it defines the point at which a sell operation will auto liquidate all remaining holdings.
	// For example if I hold 1000 nanos of creator coins and sell x nanos such that
	// 1000 - x < CreatorCoinAutoSellThresholdNanos, we auto liquidate the remaining holdings.
	// It does this to prevent issues with floating point rounding that can arise.
	// This value should be chosen such that the chain is resistant to "phantom nanos." Phantom nanos
	// are tiny amounts of CreatorCoinsInCirculation/BitCloutLocked which can cause
	// the effective reserve ratio to deviate from the expected reserve ratio of the bancor curve.
	// A higher CreatorCoinAutoSellThresholdNanos makes it prohibitively expensive for someone to
	// attack the bancor curve to any meaningful measure.
	CreatorCoinAutoSellThresholdNanos uint64

	// The most deflationary event in BitClout history has yet to come...
	DeflationBombBlockHeight uint64
}

// GenesisBlock defines the genesis block used for the BitClout maainnet and testnet
var (
	ArchitectPubKeyBase58Check = "BC1YLg3oh6Boj8e2boCo1vQCYHLk1rjsHF6jthBdvSw79bixQvKK6Qa"
	// This is the public key corresponding to the BitcoinBurnAddress on mainnet.
	BurnPubKeyBase58Check = "BC1YLjWBf2qnDJmi8HZzzCPeXqy4dCKq95oqqzerAyW8MUTbuXTb1QT"

	GenesisBlock = MsgBitCloutBlock{
		Header: &MsgBitCloutHeader{
			Version:               0,
			PrevBlockHash:         &BlockHash{},
			TransactionMerkleRoot: mustDecodeHexBlockHash("4b71d103dd6fff1bd6110bc8ed0a2f3118bbe29a67e45c6c7d97546ad126906f"),
			TstampSecs:            uint64(1610948544),
			Height:                uint64(0),
			Nonce:                 uint64(0),
		},
		Txns: []*MsgBitCloutTxn{
			{
				TxInputs: []*BitCloutInput{},
				// The outputs in the genesis block aren't actually used by anything, but
				// including them helps our block explorer return the genesis transactions
				// without needing an explicit special case.
				TxOutputs: SeedBalances,
				// TODO: Pick a better string
				TxnMeta: &BlockRewardMetadataa{
					ExtraData: []byte(
						"They came here, to the New World. World 2.0, version 1776."),
				},
				// A signature is not required for BLOCK_REWARD transactions since they
				// don't spend anything.
			},
		},
	}
	GenesisBlockHashHex = "5567c45b7b83b604f9ff5cb5e88dfc9ad7d5a1dd5818dd19e6d02466f47cbd62"
	GenesisBlockHash    = mustDecodeHexBlockHash(GenesisBlockHashHex)

	ParamUpdaterPublicKeys = map[PkMapKey]bool{
		// 19Hg2mAJUTKFac2F2BBpSEm7BcpkgimrmD
		MakePkMapKey(MustBase58CheckDecode(ArchitectPubKeyBase58Check)):                                true,
		MakePkMapKey(MustBase58CheckDecode("BC1YLiXwGTte8oXEEVzm4zqtDpGRx44Y4rqbeFeAs5MnzsmqT5RcqkW")): true,
		MakePkMapKey(MustBase58CheckDecode("BC1YLgGLKjuHUFZZQcNYrdWRrHsDKUofd9MSxDq4NY53x7vGt4H32oZ")): true,
		MakePkMapKey(MustBase58CheckDecode("BC1YLj8UkNMbCsmTUTx5Z2bhtp8q86csDthRmK6zbYstjjbS5eHoGkr")): true,
		MakePkMapKey(MustBase58CheckDecode("BC1YLgD1f7yw7Ue8qQiW7QMBSm6J7fsieK5rRtyxmWqL2Ypra2BAToc")): true,
		MakePkMapKey(MustBase58CheckDecode("BC1YLfz4GH3Gfj6dCtBi8bNdNTbTdcibk8iCZS75toUn4UKZaTJnz9y")): true,
		MakePkMapKey(MustBase58CheckDecode("BC1YLfoSyJWKjHGnj5ZqbSokC3LPDNBMDwHX3ehZDCA3HVkFNiPY5cQ")): true,
	}
)

// BitCloutMainnetParams defines the BitClout parameters for the mainnet.
var BitCloutMainnetParams = BitCloutParams{
	NetworkType:        NetworkType_MAINNET,
	ProtocolVersion:    1,
	MinProtocolVersion: 1,
	UserAgent:          "Architect",
	DNSSeeds: []string{
		"bitclout.coinbase.com",
		"bitclout.gemini.com",
		"bitclout.kraken.com",
		"bitclout.bitstamp.com",
		"bitclout.bitfinex.com",
		"bitclout.binance.com",
		"bitclout.hbg.com",
		"bitclout.okex.com",
		"bitclout.bithumb.com",
		"bitclout.upbit.com",
	},
	DNSSeedGenerators: [][]string{
		{
			"bitclout-seed-",
			".io",
		},
	},

	GenesisBlock:        &GenesisBlock,
	GenesisBlockHashHex: GenesisBlockHashHex,
	// This is used as the starting difficulty for the chain.
	MinDifficultyTargetHex: "000001FFFF000000000000000000000000000000000000000000000000000000",

	// Run with --v=2 and look for "cum work" output from miner.go
	MinChainWorkHex: "000000000000000000000000000000000000000000000000006314f9a85a949b",

	MaxTipAge: 24 * time.Hour,

	// ===================================================================================
	// Mainnet Bitcoin config
	// ===================================================================================
	BitcoinDNSSeeds: []string{
		"seed.bitcoin.sipa.be",       // Pieter Wuille, only supports x1, x5, x9, and xd
		"dnsseed.bluematt.me",        // Matt Corallo, only supports x9
		"dnsseed.bitcoin.dashjr.org", // Luke Dashjr
		"seed.bitcoinstats.com",      // Christian Decker, supports x1 - xf
		"seed.bitnodes.io",
		"seed.bitcoin.jonasschnelli.ch", // Jonas Schnelli, only supports x1, x5, x9, and xd
		"seed.btc.petertodd.org",        // Peter Todd, only supports x1, x5, x9, and xd
		"seed.bitcoin.sprovoost.nl",     // Sjors Provoost
		"dnsseed.emzy.de",               // Stephan Oeste
	},
	// The MinChainWork value we set below has been adjusted for the BitcoinStartBlockNode we
	// chose. Basically it's the work to get from the start block node we set up to the
	// current tip.
	// Run with --v=2 and look for "CumWork" output from bitcoin_manager.go
	BitcoinMinChainWorkHex: "000000000000000000000000000000000000000007e99d77f246db819c3dfa96",

	BitcoinDefaultPort:          "8333",
	BitcoinBtcdParams:           &chaincfg.MainNetParams,
	BitcoinProtocolVersion:      70013,
	BitcoinBlocksPerRetarget:    2016,
	BitcoinPowLimitBits:         0x1d00ffff,
	BitcoinMinDiffReductionTime: 0,

	BitcoinTargetTimespanSecs:      1209600,
	BitcoinMinRetargetTimespanSecs: 1209600 / 4,
	BitcoinMaxRetargetTimespanSecs: 1209600 * 4,
	// Normal Bitcoin clients set this to be 24 hours usually. The reason we diverge here
	// is because we want to decrease the amount of time a user has to wait before BitClout
	// nodes will be willing to process a BitcoinExchange transaction. Put another way, making
	// this longer would require users to wait longer before their BitcoinExchange transactions
	// are accepted, which is
	// something we want to avoid. On the other hand, if we make this time too short
	// (e.g. <10m as an extreme example), then we might think we're not current when in
	// practice the problem is just that the Bitcoin blockchain took a little longer than
	// usual to generate a block.
	//
	// As such, considering all of the above, the time we use here should be the shortest
	// time that virtually guarantees that a Bitcoin block has been generated during this
	// interval. The longest Bitcoin inter-block arrival time since 2011 was less than two
	// hours but just to be on the safe side, we pad this value a bit and call it a day. In
	// the worst-case if the Bitcoin chain stalls for longer than this, the BitClout chain will
	// just pause for the same amount of time and jolt back into life once the Bitcoin chain
	// comes back online, which is not the end of the world. In practice, we could also sever
	// the dependence of the BitClout chain on the Bitcoin chain at some point ahead of time if we
	// expect this will be an issue (remember BitcoinExchange transactions are really only
	// needed to bootstrap the initial supply of BitClout).
	//
	// See this answer for a discussion on Bitcoin block arrival times:
	// - https://www.reddit.com/r/Bitcoin/comments/1vkp1x/what_is_the_longest_time_between_blocks_in_the/
	BitcoinMaxTipAge:         4 * time.Hour,
	BitcoinTimeBetweenBlocks: 10 * time.Minute,
	// As discussed in the original comment for this field, this is actually the minimum
	// number of blocks a burn transaction must have between the block where it was mined
	// and the *StalestBitcoinBlock*, not the tip (where StalestBitcoinBlock is defined
	// in the original comment). As such, if we can presume that the StalestBitcoinBlock is
	// is generally already defined as a block with a fair amount of work built on top of it
	// then the value of BitcoinMinBurnWorkBlocks doesn't need to be very high (in fact we could
	// theoretically make it zero). However, there is a good reason to make it a substantive
	// value and that is that in situations in which the Bitcoin blockchain is producing
	// blocks with a very high time between blocks (for example due to a bad difficulty
	// mismatch), then the StalestBitcoinBlock could actually be pretty close to the Bitcoin
	// tip (it could theoretically actually *be* the tip). As such, to protect ourselves in
	// this case, we demand a solid number of blocks having been mined between the
	// StalestBitcoinTip, which we assume isn't super reliable, and any Bitcoin burn transaction
	// that we are mining into the BitClout chain.
	BitcoinMinBurnWorkBlockss:      int64(1),
	MinerBitcoinMinBurnWorkBlockss: int64(3),

	BitcoinBurnAddress: "1PuXkbwqqwzEYo9SPGyAihAge3e9Lc71b",

	// Reject Bitcoin blocks that are more than two hours in the future.
	BitcoinMaxTstampOffsetSeconds: 2 * 60 * 60,

	// We use a start node that is near the tip of the Bitcoin header chain. Doing
	// this allows us to bootstrap Bitcoin transactions much more quickly without
	// comrpomising on security because, if this node ends up not being on the best
	// chain one day (which would be completely ridiculous anyhow because it would mean that
	// days or months of bitcoin transactions got reverted), our code will still be
	// able to robustly switch to an alternative chain that has more work. It's just
	// much faster if the best chain is the one that has this start node in it (similar
	// to the --assumevalid Bitcoin flag).
	//
	// Process for generating this config:
	// - Find a node config from the test_nodes folder (we used fe0)
	// - Make sure the logging for bitcoin_manager is set to 2. --vmodule="bitcoin_manager=2"
	// - Run the node config (./fe0)
	// - A line should print every time there's a difficulty adjustment with the parameters
	//   required below (including "DiffBits"). Just copy those into the below and
	//   everything should work.
	// - Oh and you might have to set BitcoinMinChainWorkHex to something lower/higher. The
	//   value should equal the amount of work it takes to get from whatever start node you
	//   choose and the tip. This is done by running once, letting it fail, and then rerunning
	//   with the value it outputs.
	BitcoinStartBlockNode: NewBlockNode(
		nil,
		mustDecodeHexBlockHashBitcoin("000000000000000000092d577cc673bede24b6d7199ee69c67eeb46c18fc978c"),
		// Note the height is always one greater than the parent node.
		653184,
		_difficultyBitsToHash(386798414),
		// CumWork shouldn't matter.
		big.NewInt(0),
		// We are bastardizing the BitClout header to store Bitcoin information here.
		&MsgBitCloutHeader{
			TstampSecs: 1602950620,
			Height:     0,
		},
		StatusBitcoinHeaderValidated,
	),

	BitcoinExchangeFeeBasisPoints:   10,
	BitcoinDoubleSpendWaitSeconds:   5.0,
	BitCloutNanosPurchasedAtGenesis: uint64(6000000000000000),
	DefaultSocketPort:               uint16(17000),
	DefaultJSONPort:                 uint16(17001),

	DialTimeout:               30 * time.Second,
	VersionNegotiationTimeout: 30 * time.Second,

	BlockRewardMaturity: time.Hour * 3,

	V1DifficultyAdjustmentFactor: 10,

	// Use a five-minute block time. Although a shorter block time seems like
	// it would improve the user experience, the reality is that zero-confirmation
	// transactions can usually be relied upon to give the user the illusion of
	// instant gratification (particularly since we implement a limited form of
	// RBF that makes it difficult to reverse transactions once they're in the
	// mempool of nodes). Moreover, longer block times mean we require fewer
	// headers to be downloaded by light clients in the long run, which is a
	// big win in terms of performance.
	TimeBetweenBlocks: 5 * time.Minute,
	// We retarget the difficulty every day. Note this value must
	// ideally be evenly divisible by TimeBetweenBlocks.
	TimeBetweenDifficultyRetargets: 24 * time.Hour,
	// Difficulty can't decrease to below 25% of its previous value or increase
	// to above 400% of its previous value.
	MaxDifficultyRetargetFactor: 4,
	Base58PrefixPublicKey:       [3]byte{0xcd, 0x14, 0x0},
	Base58PrefixPrivateKey:      [3]byte{0x35, 0x0, 0x0},

	// Reject blocks that are more than two hours in the future.
	MaxTstampOffsetSeconds: 2 * 60 * 60,

	// We use a max block size of 16MB. This translates to 100-200 posts per
	// second depending on the size of the post, which should support around
	// ten million active users. We compute this by taking Twitter, which averages
	// 6,000 posts per second at 300M daus => 10M/300M*6,000=200 posts per second. This
	// generates about 1.6TB per year of data, which means that nodes will
	// have to have a lot of space. This seems fine, however,
	// because space is cheap and it's easy to spin up a cloud machine with
	// tens of terabytes of space.
	MaxBlockSizeBytes: 16000000,

	// We set this to be lower initially to avoid winding up with really big
	// spam blocks in the event someone tries to abuse the initially low min
	// fee rates.
	MinerMaxBlockSizeBytes: 2000000,

	// This takes about ten seconds on a reasonable CPU, which makes sense given
	// a 10 minute block time.
	MiningIterationsPerCycle: 95000,

	MaxUsernameLengthBytes: MaxUsernameLengthBytes,

	MaxUserDescriptionLengthBytes: 20000,

	MaxProfilePicLengthBytes: 20000,
	MaxProfilePicDimensions:  100,

	// MaxPrivateMessageLengthBytes is the maximum number of bytes of encrypted
	// data a private message is allowed to include in an PrivateMessage transaction.
	MaxPrivateMessageLengthBytes: 10000,

	// Set the stake fee to 10%
	StakeFeeBasisPoints: 10 * 100,
	// TODO(performance): We're currently storing posts using HTML, which
	// basically 2x as verbose as it needs to be for basically no reason.
	// We should consider storing stuff as markdown instead, which we can
	// do with the richtext editor thing that we have.
	MaxPostBodyLengthBytes: 20000,
	MaxPostSubLengthBytes:  140,
	// 10x is the max for the truly highly motivated individuals.
	MaxStakeMultipleBasisPoints: 10 * 100 * 100,
	// 100% is the max creator percentage. Not sure why you'd buy such a coin
	// but whatever.
	MaxCreatorBasisPoints:  100 * 100,
	ParamUpdaterPublicKeys: ParamUpdaterPublicKeys,

	// Use a canonical set of seed transactions.
	SeedTxns: SeedTxns,

	// Set some seed balances if desired
	SeedBalances: SeedBalances,

	// Just charge one basis point on creator coin trades for now.
	CreatorCoinTradeFeeBasisPoints: 1,
	// Note that Uniswap is quadratic (i.e. its price equation is
	// - price ~= currentCreatorCoinSupply^2,
	// and we think quadratic makes sense in this context as well.
	CreatorCoinSlope:        NewFloat().SetFloat64(0.003),
	CreatorCoinReserveRatio: NewFloat().SetFloat64(0.3333333),

	// 10 was seen as a threshold reachable in almost all transaction.
	// It's just high enough where you avoid drifting creating coin
	// reserve ratios.
	CreatorCoinAutoSellThresholdNanos: uint64(10),
}

func mustDecodeHexBlockHashBitcoin(ss string) *BlockHash {
	hash, err := chainhash.NewHashFromStr(ss)
	if err != nil {
		panic(err)
	}
	return (*BlockHash)(hash)
}

func MustDecodeHexBlockHash(ss string) *BlockHash {
	return mustDecodeHexBlockHash(ss)
}

func mustDecodeHexBlockHash(ss string) *BlockHash {
	bb, err := hex.DecodeString(ss)
	if err != nil {
		log.Fatalf("Problem decoding hex string to bytes: (%s): %v", ss, err)
	}
	if len(bb) != 32 {
		log.Fatalf("mustDecodeHexBlockHash: Block hash has length (%d) but should be (%d)", len(bb), 32)
	}
	ret := BlockHash{}
	copy(ret[:], bb)
	return &ret
}

// BitCloutTestnetParams defines the BitClout parameters for the testnet.
var BitCloutTestnetParams = BitCloutParams{
	NetworkType:        NetworkType_TESTNET,
	ProtocolVersion:    0,
	MinProtocolVersion: 0,
	UserAgent:          "Architect",
	DNSSeeds:           []string{},
	DNSSeedGenerators:  [][]string{},

	// ===================================================================================
	// Testnet Bitcoin config
	// ===================================================================================
	//
	// We use the Bitcoin testnet when we use the BitClout testnet. Note there's no
	// reason we couldn't use the Bitcoin mainnet with the BitClout testnet instead,
	// but it seems reasonable to assume someone using the BitClout testnet would prefer
	// the Bitcoin side be testnet as well.
	BitcoinDNSSeeds: []string{
		"testnet-seed.bitcoin.jonasschnelli.ch",
		"testnet-seed.bitcoin.schildbach.de",
		"seed.tbtc.petertodd.org",
		"testnet-seed.bluematt.me",
		"seed.testnet.bitcoin.sprovoost.nl",
	},
	// See comment in mainnet config.
	// Below we have a ChainWork value that is much lower to accommodate testing situations.
	//
	//BitcoinMinChainWorkHex:      "000000000000000000000000000000000000000000000000000007d007d007d0",
	BitcoinMinChainWorkHex:      "0000000000000000000000000000000000000000000000000000000000000000",
	BitcoinDefaultPort:          "18333",
	BitcoinBtcdParams:           &chaincfg.TestNet3Params,
	BitcoinProtocolVersion:      70013,
	BitcoinBlocksPerRetarget:    2016,
	BitcoinPowLimitBits:         0x1d00ffff,
	BitcoinMinDiffReductionTime: time.Minute * 20,

	BitcoinTargetTimespanSecs:      1209600,
	BitcoinMinRetargetTimespanSecs: 1209600 / 4,
	BitcoinMaxRetargetTimespanSecs: 1209600 * 4,

	// See commentary on these values in the Mainnet config above and in the struct
	// definition (also above).
	//
	// Below are some alternative settings for BitcoinMaxTipAge that are useful
	// for testing. They make it so that the chain becomes current really fast,
	// meaning things that block until the Bitcoin chain is current can be tested
	// more easily.
	//
	// Super quick age (does one header download before becoming current)
	//BitcoinMaxTipAge: 65388 * time.Hour,
	// Medium quick age (does a few header downloads before becoming current)
	//BitcoinMaxTipAge: 64888 * time.Hour,
	// TODO: Change back to 3 hours when we launch the testnet. In the meantime this value
	// is more useful for local testing.
	//BitcoinMaxTipAge:              3 * time.Hour,
	// NOTE: This must be set to a very low value if you want Bitcoin burn transactions
	// to get considered more quickly.
	BitcoinMaxTipAge:         4 * time.Hour,
	BitcoinTimeBetweenBlocks: 10 * time.Minute,
	// TODO: Change back to 6 blocks when we launch the testnet. In the meantime this value
	// is more useful for local testing.
	//BitcoinMinBurnWorkBlocks:      6, // = 60min / 10min per block
	BitcoinMinBurnWorkBlockss:      int64(1),
	MinerBitcoinMinBurnWorkBlockss: int64(3),

	BitcoinBurnAddress:              "mhziDsPWSMwUqvZkVdKY92CjesziGP3wHL",
	BitcoinExchangeFeeBasisPoints:   10,
	BitcoinDoubleSpendWaitSeconds:   5.0,
	BitCloutNanosPurchasedAtGenesis: uint64(6000000000000000),

	// Reject Bitcoin blocks that are more than two hours in the future.
	BitcoinMaxTstampOffsetSeconds: 2 * 60 * 60,

	// See comment in mainnet config.
	BitcoinStartBlockNode: NewBlockNode(
		nil,
		mustDecodeHexBlockHashBitcoin("000000000000003aae8fb976056413aa1d863eb5bee381ff16c9642283b1da1a"),
		1897056,
		_difficultyBitsToHash(424073553),

		// CumWork: We set the work of the start node such that, when added to all of the
		// blocks that follow it, it hurdles the min chain work.
		big.NewInt(0),
		// We are bastardizing the BitClout header to store Bitcoin information here.
		&MsgBitCloutHeader{
			TstampSecs: 1607659152,
			Height:     0,
		},
		StatusBitcoinHeaderValidated,
	),

	// ===================================================================================
	// Testnet socket config
	// ===================================================================================
	DefaultSocketPort: uint16(18000),
	DefaultJSONPort:   uint16(18001),

	DialTimeout:               30 * time.Second,
	VersionNegotiationTimeout: 30 * time.Second,

	GenesisBlock:        &GenesisBlock,
	GenesisBlockHashHex: GenesisBlockHashHex,

	// Use a very fast block time in the testnet.
	TimeBetweenBlocks: 2 * time.Second,
	// Use a very short difficulty retarget period in the testnet.
	TimeBetweenDifficultyRetargets: 6 * time.Second,
	// This is used as the starting difficulty for the chain.
	MinDifficultyTargetHex: "0090000000000000000000000000000000000000000000000000000000000000",
	// Minimum amount of work a valid chain needs to have. Useful for preventing
	// disk-fill attacks, among other things.
	//MinChainWorkHex: "000000000000000000000000000000000000000000000000000000011883b96c",
	MinChainWorkHex: "0000000000000000000000000000000000000000000000000000000000000000",

	// TODO: Set to one day when we launch the testnet. In the meantime this value
	// is more useful for local testing.
	MaxTipAge: time.Hour * 24,

	// Difficulty can't decrease to below 50% of its previous value or increase
	// to above 200% of its previous value.
	MaxDifficultyRetargetFactor: 2,
	// Miners need to wait some time before spending their block reward.
	// TODO: Make this 24 hours when we launch the testnet. In the meantime this value
	// is more useful for local testing.
	BlockRewardMaturity: time.Second * 4,

	V1DifficultyAdjustmentFactor: 10,

	// Reject blocks that are more than two hours in the future.
	MaxTstampOffsetSeconds: 2 * 60 * 60,

	// We use a max block size of 1MB. This seems to work well for BTC and
	// most of our data doesn't need to be stored on the blockchain anyway.
	MaxBlockSizeBytes: 1000000,

	// We set this to be lower initially to avoid winding up with really big
	// spam blocks in the event someone tries to abuse the initially low min
	// fee rates.
	MinerMaxBlockSizeBytes: 200000,

	Base58PrefixPublicKey:  [3]byte{0x11, 0xc2, 0x0},
	Base58PrefixPrivateKey: [3]byte{0x4f, 0x6, 0x1b},

	MiningIterationsPerCycle: 9500,

	// bitclout
	MaxUsernameLengthBytes: MaxUsernameLengthBytes,

	MaxUserDescriptionLengthBytes: 20000,

	MaxProfilePicLengthBytes: 20000,
	MaxProfilePicDimensions:  100,

	// MaxPrivateMessageLengthBytes is the maximum number of bytes of encrypted
	// data a private message is allowed to include in an PrivateMessage transaction.
	MaxPrivateMessageLengthBytes: 10000,

	// Set the stake fee to 5%
	StakeFeeBasisPoints: 5 * 100,
	// TODO(performance): We're currently storing posts using HTML, which
	// basically 2x as verbose as it needs to be for basically no reason.
	// We should consider storing stuff as markdown instead, which we can
	// do with the richtext editor thing that we have.
	MaxPostBodyLengthBytes: 50000,
	MaxPostSubLengthBytes:  140,
	// 10x is the max for the truly highly motivated individuals.
	MaxStakeMultipleBasisPoints: 10 * 100 * 100,
	// 100% is the max creator percentage. Not sure why you'd buy such a coin
	// but whatever.
	MaxCreatorBasisPoints:  100 * 100,
	ParamUpdaterPublicKeys: ParamUpdaterPublicKeys,

	// Use a canonical set of seed transactions.
	SeedTxns: TestSeedTxns,

	// Set some seed balances if desired
	SeedBalances: TestSeedBalances,

	// Just charge one basis point on creator coin trades for now.
	CreatorCoinTradeFeeBasisPoints: 1,
	// Note that Uniswap is quadratic (i.e. its price equation is
	// - price ~= currentCreatorCoinSupply^2,
	// and we think quadratic makes sense in this context as well.
	CreatorCoinSlope:        NewFloat().SetFloat64(0.003),
	CreatorCoinReserveRatio: NewFloat().SetFloat64(0.3333333),

	// 10 was seen as a threshold reachable in almost all transaction.
	// It's just high enough where you avoid drifting creating coin
	// reserve ratios.
	CreatorCoinAutoSellThresholdNanos: uint64(10),
}

// GetDataDir gets the user data directory where we store files
// in a cross-platform way.
func GetDataDir(params *BitCloutParams) string {
	configDirs := configdir.New(
		ConfigDirVendorName, ConfigDirAppName)
	dirString := configDirs.QueryFolders(configdir.Global)[0].Path
	dataDir := filepath.Join(dirString, params.NetworkType.String())
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		log.Fatalf("GetDataDir: Could not create data directories (%s): %v", dataDir, err)
	}
	return dataDir
}

// Defines keys that may exist in a transaction's ExtraData map
const (
	// Key in transaction's extra data map that points to a post that the current transaction is reclouting
	RecloutedPostHash = "RecloutedPostHash"
	// Key in transaction's extra map -- The presence of this key indicates that this post is a reclout with a quote.
	IsQuotedReclout = "IsQuotedReclout"

	// Keys for a GlobalParamUpdate transaction's extra data map.
	USDCentsPerBitcoin            = "USDCentsPerBitcoin"
	MinNetworkFeeNanosPerKB       = "MinNetworkFeeNanosPerKB"
	CreateProfileFeeNanos         = "CreateProfileFeeNanos"
	ForbiddenBlockSignaturePubKey = "ForbiddenBlockSignaturePubKey"

	DiamondLevelKey    = "DiamondLevel"
	DiamondPostHashKey = "DiamondPostHash"
)

// Defines values that may exist in a transaction's ExtraData map
var (
	PostExtraDataConsensusKeys = [2]string{RecloutedPostHash, IsQuotedReclout}
)

var (
	QuotedRecloutVal    = []byte{1}
	NotQuotedRecloutVal = []byte{0}
)

var (
	IsGraylisted  = []byte{1}
	IsBlacklisted = []byte{1}
)

// InitialGlobalParamsEntry to be used before ParamUpdater creates the first update.
var (
	InitialGlobalParamsEntry = GlobalParamsEntry{
		// We initialize the USDCentsPerBitcoin to 0 so we can use the value set by the UPDATE_BITCOIN_USD_EXCHANGE_RATE.
		USDCentsPerBitcoin: 0,
		// We initialize the MinimumNetworkFeeNanosPerKB to 0 so we do not assess a minimum fee until specified by ParamUpdater.
		MinimumNetworkFeeNanosPerKB: 0,
		// We initialize the CreateProfileFeeNanos to 0 so we do not assess a fee to create a profile until specified by ParamUpdater.
		CreateProfileFeeNanos: 0,
	}
)

// Define min / max possible values for GlobalParams.
const (
	// MinNetworkFeeNanosPerKBValue - Minimum value to which the minimum network fee per KB can be set.
	MinNetworkFeeNanosPerKBValue = 0
	// MaxNetworkFeeNanosPerKBValue - Maximum value to which the maximum network fee per KB can be set.
	MaxNetworkFeeNanosPerKBValue = 100 * NanosPerUnit
	// MinCreateProfileFeeNanos - Minimum value to which the create profile fee can be set.
	MinCreateProfileFeeNanos = 0
	// MaxCreateProfileFeeNanos - Maximum value to which the create profile fee can be set.
	MaxCreateProfileFeeNanos = 100 * NanosPerUnit
)