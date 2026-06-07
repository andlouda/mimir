package main

// AppVersion is overridden by release builds via ldflags:
// -X main.AppVersion=0.1.0 -X main.UpdateRepository=owner/repo
var AppVersion = "0.0.0-dev"

// UpdateRepository is the GitHub repository used for update checks.
// Keep empty until the public GitHub repository exists.
var UpdateRepository = ""
