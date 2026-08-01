package services

import "errors"

//this file will hold constants used throughout the services section.

//change this value will allow you to change the minimum number of hours can be set for a service sync schedule on the back end (does not effect the UI)
const MinScheduleInterval = 12

// ErrCredentialTestFailed indicates AddService rejected credentials because
// the service reported them invalid (e.g. wrong password, expired token),
// as opposed to an internal failure while storing them. Callers can check
// for it with errors.Is to pick an appropriate response.
var ErrCredentialTestFailed = errors.New("credential test failed")

// ErrSyncInProgress indicates a sync was already running for a user+service
// when a new one was requested, as opposed to the sync itself failing.
var ErrSyncInProgress = errors.New("sync already in progress")

// ErrServiceNotConnected indicates the requested user has no stored
// credentials/entry for a service, as opposed to an internal lookup failure.
var ErrServiceNotConnected = errors.New("service not connected")

//This is the name used on the backend for the pixiv service, any api request accessing this service must match this string to identify the service
const pixivServiceName = "pixiv"

const PixivDelaySec = 1.0 //seconds between requests
const PixivQpT = 1        //number of queries between delays
