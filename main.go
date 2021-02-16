package main

import (
  "fmt"
  "bytes"
  "time"
  
  "github.com/ethereum/go-ethereum/trie"
  "github.com/ethereum/go-ethereum/rlp"
  "github.com/ethereum/go-ethereum/common"
  "github.com/ethereum/go-ethereum/log"
  "github.com/ethereum/go-ethereum/core/state"
  "github.com/ethereum/go-ethereum/core/rawdb"
  "github.com/ethereum/go-ethereum/crypto"
)

func main() {
  
  var emptyCodeHash = crypto.Keccak256(nil)
  
  //read in database for tenant to be migrated
  path := "/Users/angelapratt/projects/quorum-creator/network/7-nodes-istanbul-tessera-bash/qdata/dd2/geth/chaindata"
  diskdb, _ := rawdb.NewLevelDBDatabase(path, 0, 0, "")
  
  //create trie database from leveldb
  triedb := trie.NewDatabase(diskdb)
  
  blockNum := 127
  
  //each iteration takes about ~200-500ms each for between ~100-500 creation txs
  //test doing sequentially (db caching) vs in parallel
  totalTimeStart := time.Now()
  
  for i := 1; i < blockNum; i++ {

    //get blockhashes for block X and X-1
    blockHash1 := rawdb.ReadBlock(diskdb, rawdb.ReadCanonicalHash(diskdb, uint64(i-1)), uint64(i-1)).Root()
    blockHash2 := rawdb.ReadBlock(diskdb, rawdb.ReadCanonicalHash(diskdb, uint64(i)), uint64(i)).Root()
  
    //get private state roots for block x and x+1
    root1, _ := diskdb.Get(append([]byte("P"), blockHash1[:]...))
    root2, _ := diskdb.Get(append([]byte("P"), blockHash2[:]...))
    firstRoot := common.BytesToHash(root1)
    secondRoot := common.BytesToHash(root2)
    fmt.Println(firstRoot)
    fmt.Println(secondRoot)
  
    //get the tries
    trie1, _ := trie.NewSecure(firstRoot, triedb)
    trie2, _ := trie.NewSecure(secondRoot, triedb)
  
    //get the corresponding iterators (starting at root)
    it1 := trie1.NodeIterator(nil)
    it2 := trie2.NodeIterator(nil)
    
    //create maps/lists to store the changed accounts and their addresses
    //will use these to add to proper states in PrivateStateService
    var allAddresses = make([]common.Address, 0)
    var emptyAddresses = make(map[common.Address]state.Account)
    var addresses = make(map[common.Address]state.Account)
  
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
      firstAccount := itDiff.LeafBlob()
      var data state.Account
      if err := rlp.DecodeBytes(firstAccount, &data); err != nil {
        log.Error("Failed to decode state object", "err", err)
      }
    
      //collect the address
      //empty map doesnt actually need the Account, because will be inserted in the EmptyState at the address as an Empty Account
      allAddresses = append(allAddresses, firstAddress)
      emptyAddresses[firstAddress] = data
    
      //check if the Account is not empty (has code associated with it)
      //will pick up user created contracts and contracts created by other contracts
      if(!bytes.Equal(data.CodeHash, emptyCodeHash)) {
        addresses[firstAddress] = data
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
        emptyAddresses[address] = data
      
        if(!bytes.Equal(data.CodeHash, emptyCodeHash)) {
          addresses[address] = data
        }
      }
    }
  
    duration := time.Since(start)
  
    //print out information for now
    //in future will add these accounts to managedStates(empty, private or psi) to newly created or retrieved privateStateService on upgraded quorum
  
    fmt.Println("duration", duration)
  
    fmt.Println("numtxs", len(allAddresses))
  
    // for _, acc := range allAddresses {
    //   fmt.Println("acc", acc)
    // }
    // fmt.Println("------------")
    // 
    // for k, _ := range emptyAddresses {
    //   fmt.Println("empty", k)
    // }
    // fmt.Println("------------")
    // 
    // for k, v := range addresses {
    //   fmt.Println("keep", k, "acc", v)
    // }
  }
  
  totalDuration := time.Since(totalTimeStart)
  fmt.Println("TOTAL DURATION:", totalDuration)
}