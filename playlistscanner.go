package main

import (
	"io"
	"fmt"
	"log"
	"time"
	"strconv"
	"net/http"
	"encoding/hex"
	"encoding/json"

	bolt "go.etcd.io/bbolt"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	//"github.com/ipld/go-ipld-prime/node/basicnode"
  //"github.com/ipld/go-ipld-prime/codec/dagcbor"
)

var bRunning = false

func StartScanner() {

	fmt.Printf("Scanner started...\n")
	bRunning = true

	isFirstIter := true

	accountsMissing := 0
	accountsComplete := 0
	iterAccount := make([]byte, 20)
	lastAccount := make([]byte, 20)

	for bRunning {
		tx, _ := db.Begin(false)
		b := tx.Bucket([]byte("accounts"))
		cu := b.Cursor()
		k, v := cu.Seek(lastAccount)
		if len(k) == 0 {
			// no accounts at all?
			bRunning = false
			tx.Rollback()
			break
		} else if isFirstIter {
			isFirstIter = false
		} else {
			k, v = cu.Next()
		}
		if len(k) == 0 {
			// end reached
			isFirstIter = true
			lastAccount = make([]byte, 20)			

			if accountsMissing == 0 {
				bRunning = false
				tx.Rollback()
				break
			}

			if accountsMissing == accountsComplete {
				bRunning = false
				tx.Rollback()
				break
			}

			if accountsMissing > accountsComplete {
				time.Sleep(5 * time.Second)
				accountsComplete = 0
				accountsMissing = 0
				continue
			}
		}
		copy(iterAccount, k)
		iterVideoBytes := make([]byte, len(v))
		copy(iterVideoBytes, v)
		tx.Rollback()

		//fmt.Printf("iterAccount = %+v\n", iterAccount)			
		//fmt.Printf(".")

		acct := AccountStruct{}
		acct.Unmarshal(v)

		if acct.HasMissing {
			accountsMissing++
			if acct.PlaylistRootCid != "" {
				//tmpCid, _ := cid.Parse(acct.PlaylistRootCid)
				tmpAcctHex := hex.EncodeToString(k)
				fmt.Printf("scanning %s\n", tmpAcctHex)
				hasMissing := ScanPlaylist(tmpAcctHex, acct.PlaylistRootCid, acct.SuggestedGateways, "")
				if hasMissing == false {
					accountsComplete++
					acct.HasMissing = false
					tx, err := db.Begin(true)
					if err != nil {
						log.Fatal(err)
					}
					b := tx.Bucket([]byte("accounts"))
					b.Put(iterAccount, acct.Marshal())
					tx.Commit()
				}
			}
		}

		lastAccount = iterAccount
		//time.Sleep(1 * time.Second)
	}

	fmt.Printf("Scanner stopped\n")
}

func ScanPlaylist(owner string, targetCid string, suggestedGateways []string, indexPrefix string) (hasMissing bool) {
	
	gw := "http://localhost:8080/ipfs/"
	if len(suggestedGateways) > 0 {
		gw = suggestedGateways[0]
	}
	resp, err := http.Get(gw + targetCid)
	if err != nil || resp.StatusCode != 200 {
		fmt.Printf("failed to fetch cid from gateway\n")
		return true
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

  nd, err := cbornode.Decode(body, mh.SHA2_256, mh.DefaultLengths[mh.SHA2_256])
  if err != nil {
    log.Printf("error cbornode.Decode: %+v\n", err)
    return false
  }

  rootDocIface, _, _ := nd.Resolve(nil)
	rootDoc := rootDocIface.(map[string]interface{})

	bHasMissing := false
	var tx *bolt.Tx
	var b *bolt.Bucket
	for k, v := range(rootDoc) {
		if fmt.Sprintf("%T", v) == "cid.Cid" {
			//fmt.Printf("recurse into cid\n")
			tmpCid := v.(cid.Cid)
			tmpHasMissing := ScanPlaylist(owner, tmpCid.String(), suggestedGateways, indexPrefix + k)
			if tmpHasMissing {
				bHasMissing = true
			}
		} else {
			// set up the db on first loop (bit of a code smell)
			if tx == nil {
				fmt.Printf("begin tx\n")
				tx, err = db.Begin(true)
				if err != nil {
					log.Fatal(err)
				}
				b = tx.Bucket([]byte("videos"))
				defer func(){
					fmt.Printf("end tx\n")
					tx.Commit()
				}()
			}

			vidId, _ := strconv.Atoi(indexPrefix + k)
			indexKey := owner + "-" + strconv.Itoa(vidId)

			//fmt.Printf("%+v\n", v)
			videoData := v.(map[string]interface{})

			thumb := videoData["thumb"].(cid.Cid)
			videoData["thumb"] = thumb.String()

			didNickname := ""
			videoData["owner"] = "0x" + owner
			if len(didNickname) > 0 {
				videoData["name"] = didNickname
			}			

			//fmt.Printf("indexing %+v\n", indexObj)
			searchIndex.Index(indexKey, videoData)
			videoBytes, _ := json.Marshal(videoData)
			b.Put([]byte(indexKey), videoBytes)
			fmt.Printf("added video to index %s\n", indexKey)
		}
	}

	return bHasMissing
}