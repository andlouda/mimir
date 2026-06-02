package main

import "testing"

func TestStrictUnmarshalJSONRejectsUnknownFields(t *testing.T) {
	var target struct {
		Name string `json:"name"`
	}

	err := strictUnmarshalJSON(`{"name":"ok","unknown":true}`, &target)
	if err == nil {
		t.Fatal("expected strict decoder to reject unknown fields")
	}
}
