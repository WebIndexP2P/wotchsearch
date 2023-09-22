package main

import (
  "os"
  "fmt"
  "log"
  "time"
  "strconv"
  "net/http"
  //"encoding/json"
  "encoding/base64"
  "encoding/hex"

  bolt "go.etcd.io/bbolt"
  "github.com/blevesearch/bleve/v2"

  "github.com/ipfs/go-cid"
  mh "github.com/multiformats/go-multihash"
  "github.com/ipfs/go-ipld-cbor"
  "github.com/ethereum/go-ethereum/crypto"
  "github.com/ethereum/go-ethereum/common"

  "code.wip2p.com/mwadmin/wip2p-go/peer"
  "code.wip2p.com/mwadmin/wip2p-go/peermanager"
  "code.wip2p.com/mwadmin/wip2p-go/clientsession"
  "code.wip2p.com/mwadmin/wip2p-go/clientsession/messages"
  "code.wip2p.com/mwadmin/wip2p-go/core/globals"
)

var db *bolt.DB
var searchIndex bleve.Index

func main() {

  clientsession.PeerManager_HaveFullNodeSession = peermanager.HaveFullNodeSession

  clientsession.OnBroadcast = func(session *clientsession.Session, msgType string, data []interface{}) {
    //fmt.Printf("OnBroadcast %+v %+v\n", msgType, data)
    params, _ := data[0].(map[string]interface{})
    rootDoc := DecodeRootBundleIface(data[0])
    //fmt.Printf("broadcast\n")
    //fmt.Printf("%+v\n", params)
    //fmt.Printf("%+v\n", rootDoc)

    tx, _ := db.Begin(true)
    EraseAllForAccount(tx, params["account"].(string))
    go func(){
      wotchDoc := ParseWotchRootDoc(session, rootDoc)
      if wotchDoc == nil {
        fmt.Printf("%s: No wotch data\n", params["account"].(string))
      } else {
        fmt.Printf("%s: Found wotch data\n", params["account"].(string))
        ProcessWotchDoc(tx, params["account"].(string), wotchDoc)
      }

      activeSeqNo := params["seqNo"].(float64)
      cfgBucket, _ := tx.CreateBucketIfNotExists([]byte("config"))
      cfgBucket.Put([]byte("lastSeqNo"), []byte(strconv.FormatUint(uint64(activeSeqNo), 10)))

      tx.Commit()
      //fmt.Printf("broadcast db commit complete\n")
    }()
  }

  /*clientsession.OnEnd = func(session *clientsession.Session) {
    fmt.Printf("ended\n")
    time.Sleep(5 * time.Second)
    StartSession()
  }*/

  globals.AppName = "wotchsearch"
  globals.AppVersion = "0.0.1"

  dbname := "wotchsearch.db"

  // bring up the db
  var err error
  db, err = bolt.Open(dbname, 0600, nil)
  if err != nil {
    log.Fatal(err)
  }

  if _, err := os.Stat("wotchsearch.bleve"); err != nil {
    fmt.Printf("Creating text index\n")
    mapping := bleve.NewIndexMapping()
    searchIndex, err = bleve.New("wotchsearch.bleve", mapping)
    if err != nil {
      panic(err)
    }    
  } else {
    searchIndex, _ = bleve.Open("wotchsearch.bleve")
    docCount, _ := searchIndex.DocCount()
    fmt.Printf("Opened search index with %d\n", docCount)
  }

  //searchIndex.Index("1234", 123)

  tx, err := db.Begin(true)

  cfgBucket, err := tx.CreateBucketIfNotExists([]byte("config"))
  if err != nil {
    log.Fatal(err)
  }

  privKey := cfgBucket.Get([]byte("privateKey"))
  if privKey == nil {
    account, _ := crypto.GenerateKey()
    address := crypto.PubkeyToAddress(account.PublicKey)
    fmt.Printf("Generated new account: %s\n", address.String())
    cfgBucket.Put([]byte("privateKey"), crypto.FromECDSA(account))
    globals.NodePrivateKey = account
  } else {
    account, _ := crypto.ToECDSA(privKey);
    address := crypto.PubkeyToAddress(account.PublicKey)
    fmt.Printf("Using account: %+v\n", address.String());
    globals.NodePrivateKey = account
  }

  // account [20]byte = [cellKeys,...]
  tx.CreateBucketIfNotExists([]byte("accounts"))
  tx.CreateBucketIfNotExists([]byte("videos"))

  if len(os.Args) > 1 && os.Args[1] == "-dump" {
    DbDump(tx)
    tx.Commit()
    return
  }

  tx.Commit()
  //globals.DebugLogging = true

  globals.PublicMode = true
  globals.RootAccount = common.HexToAddress("0x388D22ba6F190762b1dc5A813B845065b50c7da8")

  go StartSession()
  //go StartScanner()

  http.HandleFunc("/", serveHttpApi)
  http.ListenAndServe(":7777", nil)
}

func StartSession() {

  fmt.Printf("StartSession\n")

  tx, err := db.Begin(true)

  cfgBucket := tx.Bucket([]byte("config"))
  if err != nil {
    log.Fatal(err)
  }

  sequenceSeed := uint(0)
  activeSeqNo := uint(0)

  session := clientsession.Create()

  p := peer.ParseEndpointString("ws://127.0.0.1:9472")
  session.RemoteEndpoint = &p
  session.DontValidateAndSave = true
  session.OnError = func(err error) {
    fmt.Printf(err.Error() + "\n")
  }  
  session.OnEnd = func() {
    fmt.Printf("s.OnEnd\n")
    tx.Commit()
    go func() {
      time.Sleep(5 * time.Second)
      go StartSession()
    }()
  }

  err = session.Dial()
  if err != nil {
    fmt.Printf("%+v\n", err)
    return
  }

  session.StartAuthProcess()

  if !session.HasAuthed {
    fmt.Printf("HasAuthed = false, something gone wrong")
    return
  }

  // load from db
  cfgBucket, err = tx.CreateBucketIfNotExists([]byte("config"))
  bLastSeqNo := cfgBucket.Get([]byte("lastSeqNo"))
  if bLastSeqNo != nil {
    activeSeqNo_uint64, _ := strconv.ParseUint(string(bLastSeqNo), 10, 32)
    activeSeqNo = uint(activeSeqNo_uint64)
    log.Printf("Loaded Sequence No %d from Db\n", activeSeqNo)
  }

  bSeqSeed := cfgBucket.Get([]byte("sequenceSeed"))
  if bSeqSeed != nil {
    seqSeed_uint64, _ := strconv.ParseUint(string(bSeqSeed), 10, 32)
    sequenceSeed = uint(seqSeed_uint64)
    log.Printf("Loaded Sequence Seed %d from Db\n", sequenceSeed)
  }

  info, err := session.StartInfo()
  //fmt.Printf("%+v\n", info)

  // check the sequence seeds match
  if sequenceSeed != info.SequenceSeed {
    fmt.Printf("remote node has reset sequence seed\n")
    activeSeqNo = 0
    cfgBucket.Put([]byte("sequenceSeed"), []byte(strconv.FormatUint(uint64(info.SequenceSeed), 10)))
  }

  tx.Commit()

  if activeSeqNo >= info.LatestSequenceNo {
    log.Printf("We're already up to date\n")
  }

  for activeSeqNo < info.LatestSequenceNo {
    results := FetchNextSequenceBatch(session, activeSeqNo)
    //fmt.Printf("%+v\n\n", results)
    tx, err := db.Begin(true)
    if err != nil {
      log.Fatal(err)
    }

    for a := 0; a < len(results); a++ {
      activeSeqNo = results[a].SeqNo

      EraseAllForAccount(tx, results[a].Account)

      if results[a].Removed {
        continue
      }

      wotchDoc := FetchWotchDocViaBundle(session, results[a].Account)
      if wotchDoc == nil {
        //fmt.Printf("%s: No wotch data\n", results[a].Account)
      } else {
        fmt.Printf("%s: Found wotch data\n", results[a].Account)
        ProcessWotchDoc(tx, results[a].Account, wotchDoc)
      }
    }

    cfgBucket, err := tx.CreateBucketIfNotExists([]byte("config"))
    cfgBucket.Put([]byte("lastSeqNo"), []byte(strconv.FormatUint(uint64(activeSeqNo), 10)))

    tx.Commit()

    //activeSeqNo++
  }

  fmt.Printf("processing finished\n")
  if !bRunning {
    go StartScanner()
  }
}

func EraseAllForAccount(tx *bolt.Tx, account string) {
  //fmt.Printf("EraseAllForAccount %s\n", account)
}

func FetchNextSequenceBatch(session *clientsession.Session, startSeqNo uint) []messages.SequenceListItem {
  w := make(chan([]messages.SequenceListItem))
  session.SendRPC("bundle_getBySequence", []interface{}{startSeqNo}, func(result interface{}, err error){
    if err != nil {
      log.Fatal(err)
    }

    results := make([]messages.SequenceListItem, 0)
    resultArray := result.([]interface{})
    for a := 0; a < len(resultArray); a++ {
      seqListItem, err := messages.ParseSequenceListItem(resultArray[a])
      //fmt.Printf("%s\n", seqListItem)
      if err == nil {
        results = append(results, seqListItem)
      }
    }

    w <- results
  })
  return <- w
}

func FetchWotchDocViaBundle(session *clientsession.Session, account string) map[string]interface{} {
  w := make(chan(interface{}))
  params := map[string]interface{}{"account": account}
  session.SendRPC("bundle_get", []interface{}{params}, func(result interface{}, err error){
    //fmt.Printf("%+v\n", result)
    w <- DecodeRootBundleIface(result)
  })
  wotchDocIface := <- w

  return ParseWotchRootDoc(session, wotchDocIface)
}

func ParseWotchRootDoc(session *clientsession.Session, wotchDocIface interface{}) map[string]interface{} {
  //fmt.Printf("ParseWotchRootDoc\n")
  var wotchDoc map[string]interface{}
  if fmt.Sprintf("%T", wotchDocIface) == "cid.Cid" {
    tmpCid := wotchDocIface.(cid.Cid)
    wotchDocIface = FetchDoc(session, &tmpCid)
    wotchDoc, _ = wotchDocIface.(map[string]interface{})      
  } else if fmt.Sprintf("%T", wotchDocIface) == "map[string]interface {}" {
    wotchDoc = wotchDocIface.(map[string]interface{})
  }
  return wotchDoc
}

func DecodeRootBundleIface(bundleIface interface{}) interface{} {
  //fmt.Printf("DecodeRootBundleIface %+v\n", bundleIface)
  bundle, err := messages.ParseBundle(bundleIface)
  if err != nil {
    log.Fatal(err)
  }

  if len(bundle.CborData) == 0 {
    log.Printf("node cborData\n")
    return nil
  }

  bin, err := base64.StdEncoding.DecodeString(bundle.CborData[0])
  nd, err := cbornode.Decode(bin, mh.SHA2_256, mh.DefaultLengths[mh.SHA2_256])
  if err != nil {
    log.Printf("error cbornode.Decode: %+v\n", err)
    return nil
  }

  rootDocIface, _, _ := nd.Resolve(nil)
  //fmt.Printf("%+v\n", rootDocIface)
  rootDoc, _ := rootDocIface.(map[string]interface{})
  if rootDoc == nil {
    return nil
  }

  wotchRootIface, _ := rootDoc["wotch"]
  
  //return freedomcells
  return wotchRootIface
}

func ProcessWotchDoc(tx *bolt.Tx, account string, wotchDoc map[string]interface{}) {
  //fmt.Printf("ProcessWotchDoc for %s\n", account)

  tmpAccount := AccountStruct{}
  tmpAccount.ParseWotchRoot(wotchDoc)
  b := tx.Bucket([]byte("accounts"))
  bAcct, _ := hex.DecodeString(account[2:])
  b.Put(bAcct, tmpAccount.Marshal())

  //fmt.Printf("Account = %s\n", tmpAccount)
}

func FetchDoc(session *clientsession.Session, targetCid *cid.Cid) interface{} {
  //fmt.Printf("FetchDoc %s\n", targetCid.String())
  w := make(chan(interface{}))
  err := session.SendRPC("doc_get", []interface{}{targetCid.String()}, func(result interface{}, err error){
    //fmt.Printf("doc_get result -> %+v\n", result)

    bin, err := hex.DecodeString(result.(string)[2:])
    nd, err := cbornode.Decode(bin, mh.SHA2_256, mh.DefaultLengths[mh.SHA2_256])
    if err != nil {
      log.Printf("error cbornode.Decode: %+v\n", err)
      w <- nil
      return
    }

    docIface, _, _ := nd.Resolve(nil)
    w <- docIface
  })
  if err != nil {
    log.Fatal(err)
  }
  //fmt.Printf("FetchDoc end\n")
  return <- w
}
