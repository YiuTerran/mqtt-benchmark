package main

import (
	"testing"
)

func TestClient_fillPayload(t *testing.T) {
	c := &Client{Payload: map[string]interface{}{
		"createTs": "${createTs}",
		"data":     "${rand10}",
	}}

	t.Log(string(c.fillPayload()))
}
