package main

import (
  "fmt"
  "encoding/hex"
  bolt "go.etcd.io/bbolt"
)

func DbDump(tx *bolt.Tx) {
  fmt.Println("Accounts:")
  b := tx.Bucket([]byte("accounts"))
  b.ForEach(func(k, v []byte) error {
    fmt.Printf("key=%s, value=%s\n", "0x" + hex.EncodeToString(k), v)
    return nil
  })
  fmt.Println()


  fmt.Println("Config:")
  b = tx.Bucket([]byte("config"))
  b.ForEach(func(k, v []byte) error {
    fmt.Printf("key=%s, value=%s\n", k, v)
    return nil
  })
  fmt.Println()
}
