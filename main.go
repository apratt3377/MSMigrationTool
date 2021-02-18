package main

import (
  "fmt"
  "bytes"
  "time"
  
  "github.com/ethereum/go-ethereum/trie"
  "github.com/ethereum/go-ethereum/rlp"
  "github.com/ethereum/go-ethereum/common"
  "github.com/ethereum/go-ethereum/log"
  "github.com/ethereum/go-ethereum/core"
  "github.com/ethereum/go-ethereum/core/state"
  "github.com/ethereum/go-ethereum/core/rawdb"
  "github.com/ethereum/go-ethereum/crypto"
  //"github.com/ethereum/go-ethereum/core/vm"
)

//TODOS
//1. Create privateStateService at block 0 (assume migrating to a fresh node)
//2. In for loop, use privateStateService at parent block to add to managedStates
//3. Simple cli for user input (upgrade blockNum start)
//4. Cleanup, separate into useful functions
func main() {
  
  var emptyCodeHash = crypto.Keccak256(nil)
  
  //read in database for tenant to be migrated
  path := "/Users/angelapratt/projects/quorum-creator/network/3-nodes-raft-tessera-bash/qdata/dd1/geth/chaindata"
  diskdb, _ := rawdb.NewLevelDBDatabase(path, 0, 0, "")
  
  //create blockchain with only necessary components
  chainConfig := rawdb.ReadChainConfig(diskdb, rawdb.ReadCanonicalHash(diskdb, 0))
  bc := core.NewBlockChainBare(diskdb, chainConfig)
  
  //how do we determine psi if its different than the default "private"
  tenantPSI := "private"
  
  //create trie database from leveldb
  triedb := trie.NewDatabase(diskdb)
  
  blockNum := 2
  
  //each iteration takes about ~200-500ms each for between ~100-500 creation txs
  //test doing sequentially (db caching) vs in parallel
  totalTimeStart := time.Now()
  
  for i := 1; i < blockNum; i++ {

    //get blockhashes for block X and X-1
    blockHash1 := rawdb.ReadBlock(diskdb, rawdb.ReadCanonicalHash(diskdb, uint64(i-1)), uint64(i-1)).Root()
    blockX := rawdb.ReadBlock(diskdb, rawdb.ReadCanonicalHash(diskdb, uint64(i)), uint64(i))
    blockHash2 := blockX.Root()
    //get private state roots for block x and x+1
    firstRoot := rawdb.GetPrivateStateRoot(diskdb,blockHash1)
    secondRoot := rawdb.GetPrivateStateRoot(diskdb,blockHash2)
    fmt.Println(firstRoot.Hex())
    fmt.Println(secondRoot.Hex())
  
    //get the tries
    trie1, _ := trie.NewSecure(firstRoot, triedb)
    trie2, _ := trie.NewSecure(secondRoot, triedb)
  
    //get the corresponding iterators (starting at root)
    it1 := trie1.NodeIterator(nil)
    it2 := trie2.NodeIterator(nil)
    
    //create maps/lists to store the changed accounts and their addresses
    //will use these to add to proper states in PrivateStateService
    var allAddresses = make([]common.Address, 0)
    
    //privateStateService logic    
    privateStateManager, _ := core.NewMultiplePrivateStateManager(bc, blockHash1)
    emptyState, _ := privateStateManager.GetDefaultState()
    privateState, _ := privateStateManager.GetPrivateState(tenantPSI)
  
    //create the difference iterator
    //iterates over nodes in trie2 that aren't in trie1
    //when a new account gets added to trie2, it will obviously not be in trie1
    //when an existing account in trie1 gets updated, the node hash will be different and thus picked up by this iterator
    itDiff, _ := trie.NewDifferenceIterator(it1, it2)
  
    //time this process
    start := time.Now()
  
    //check if first node at iterator is leaf
    //only care about leaves because this is where the Account objects are
    //difference iterator will skip subtrees if node hash from trie1 and trie2 are the same (means no accounts have changed in that part of trie)
    if (itDiff.Leaf()) {
      //get address of Account from leaf key
      //remember: key to leaf node is the encoded address of the account
      firstAddress := common.BytesToAddress(trie2.GetKey(itDiff.LeafKey()))
    
      //get account and decode to Account Object
      //how to handle any extraData???
      firstAccount := itDiff.LeafBlob()
      var data state.Account
      if err := rlp.DecodeBytes(firstAccount, &data); err != nil {
        log.Error("Failed to decode state object", "err", err)
      }
    
      //collect the address
      //empty map doesnt actually need the Account, because will be inserted in the EmptyState at the address as an Empty Account
      allAddresses = append(allAddresses, firstAddress)
      emptyState.CreateEmptyAccount(firstAddress)
    
      //check if the Account is not empty (has code associated with it)
      //will pick up user created contracts and contracts created by other contracts
      if(!bytes.Equal(data.CodeHash, emptyCodeHash)) {
        privateState.CopyAccount(firstAddress, data)
      }
    }
  
    //loop thru the iterator and get all the other leaf nodes
    for itDiff.Next(false) {
      if (itDiff.Leaf()) {
        address := common.BytesToAddress(trie2.GetKey(itDiff.LeafKey()))
        account := itDiff.LeafBlob()
        var data state.Account
        if err := rlp.DecodeBytes(account, &data); err != nil {
          log.Error("Failed to decode state object", "err", err)
        }
        allAddresses = append(allAddresses, address)
        emptyState.CreateEmptyAccount(address)
      
        if(!bytes.Equal(data.CodeHash, emptyCodeHash)) {
          privateState.CopyAccount(address, data)
        }
      }
    }
    
    fmt.Println("emptyState", emptyState)
    fmt.Println("addr exists empty", allAddresses[0], emptyState.Exist(allAddresses[0]))
    fmt.Println("addr exists private", allAddresses[0], privateState.Exist(allAddresses[0]))
  
    duration := time.Since(start)
    
  
    fmt.Println("duration", duration)
  
    fmt.Println("numtxs", len(allAddresses))
    
    //write and commit privateStateManager
    privateStateManager.CommitAndWrite(blockX)
  }
  
  totalDuration := time.Since(totalTimeStart)
  fmt.Println("TOTAL DURATION:", totalDuration)
}