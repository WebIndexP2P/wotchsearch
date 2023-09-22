package main

import (
	"log"
	//"fmt"
	"encoding/json"
)

type AccountStruct struct {
	PlaylistRootCid string
	SuggestedGateways []string
	VideoIds []int
	HasMissing bool
}

func (a *AccountStruct) ParseWotchRoot(doc map[string]interface{}) {

	//fmt.Printf("%+v\n", doc)

	playlistRootCidIface := doc["playlist_ipfs"]
	if playlistRootCidIface != nil {
		a.PlaylistRootCid = playlistRootCidIface.(string)
		a.HasMissing = true
	}

	suggestedGatewaysIface := doc["gws"]
	if suggestedGatewaysIface != nil {
		for _, gw := range(suggestedGatewaysIface.([]interface{})) {
			a.SuggestedGateways = append(a.SuggestedGateways, gw.(string))
		}
	}
}

func (a *AccountStruct) Marshal() []byte {
	tmpBytes, err := json.Marshal(a)
	if err != nil {
		log.Fatal(err)
	}
	return tmpBytes
}

func (a *AccountStruct) Unmarshal(data []byte) error {

	err := json.Unmarshal(data, a)
	if err != nil {
		return err
	}

	return nil
}