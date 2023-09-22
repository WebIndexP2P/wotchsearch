package main

import (
  //"fmt"
  //"errors"
  "strings"
  "strconv"
  "net/http"
  "encoding/hex"
  "encoding/json"  

  "github.com/blevesearch/bleve/v2"
)

func serveHttpApi(w http.ResponseWriter, r *http.Request) {

  uriParts := strings.Split(r.URL.Path, "/")

  if len(uriParts) <= 2 {
    http.Error(w, "invalid path", 400)
    return
  }

  if uriParts[1] != "api" {
    http.Error(w, "invalid path", 400)
    return
  }

  var result []byte

  if uriParts[2] == "search" {
    var err error
    result, err = DoSearch(r)
    if err != nil {
      http.Error(w, err.Error(), 400)
      return
    }    
  } else {
    http.Error(w, "unknown method", 400)
    return
  }

  w.Header().Set("Content-Type", "application/json")
  w.Header().Set("Access-Control-Allow-Origin", "*")
  w.Write(result)
}



func DoSearch(r *http.Request) ([]byte, error) {
  //fmt.Printf("dosearch\n")
  queryValues := r.URL.Query()
    
  //videos := make([]interface{}, 0)
  qs := queryValues["q"]
  q := ""
  if len(qs) > 0 {
    q = qs[0]
  }

  pageString := queryValues["page"]
  page := 0
  if len(pageString) > 0 {
    page, _ = strconv.Atoi(pageString[0])
    if page > 0 {
      page--
    }
  }

  resultsPerPage := 12

  query := bleve.NewMatchQuery(q)
  search := bleve.NewSearchRequestOptions(query, resultsPerPage, page * resultsPerPage, false)
  search.SortBy([]string{"-timestamp"})
  searchResults, err := searchIndex.Search(search)
  if err != nil {
    return nil, err
  }

  if searchResults.Total == 0 {
    query2 := bleve.NewPrefixQuery(q)
    search := bleve.NewSearchRequestOptions(query2, resultsPerPage, page * resultsPerPage, false)
    search.SortBy([]string{"-timestamp"})
    searchResults, err = searchIndex.Search(search)
    if err != nil {
      return nil, err
    }
  }

  if searchResults.Total == 0 {
    query3 := bleve.NewFuzzyQuery(q)
    search := bleve.NewSearchRequestOptions(query3, resultsPerPage, page * resultsPerPage, false)
    search.SortBy([]string{"-timestamp"})
    searchResults, err = searchIndex.Search(search)
    if err != nil {
      return nil, err
    }
  }

  //fmt.Printf("%+v\n", searchResults.Total)
  //fmt.Printf("%+v\n", len(searchResults.Hits))

  tx, _ := db.Begin(false)
  defer tx.Rollback()
  
  a := tx.Bucket([]byte("accounts"))
  b := tx.Bucket([]byte("videos"))

  videos := make([]interface{}, 0)    
  gateways := map[string][]string{}
  for _, hit := range(searchResults.Hits) { 
    // fetch from boltdb video doc
    videoDoc := b.Get([]byte(hit.ID))    
    if videoDoc != nil {
      ownerIdPair := strings.Split(hit.ID, "-")
      ownerAddress := "0x" + ownerIdPair[0]
      if _, bExists := gateways[ownerAddress]; !bExists {        
        ownerBytes, _ := hex.DecodeString(ownerIdPair[0])
        acctBytes := a.Get(ownerBytes)
        acct := AccountStruct{}
        acct.Unmarshal(acctBytes)
        gateways[ownerAddress] = acct.SuggestedGateways
      }
      tmpObj := map[string]interface{}{}
      json.Unmarshal(videoDoc, & tmpObj)
      videos = append(videos, tmpObj)
    }
  }
  

  resultObj := map[string]interface{}{}
  resultObj["resultCount"] = searchResults.Total
  resultObj["videos"] = videos
  resultObj["gateways"] = gateways

  result, _ := json.Marshal(resultObj)
  return result, nil
}