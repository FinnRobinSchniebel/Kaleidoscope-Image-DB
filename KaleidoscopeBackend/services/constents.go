package services

//this file will hold constants used throughout the services section.

//change this value will allow you to change the minimum number of hours can be set for a service sync schedule on the back end (does not effect the UI)
const MinScheduleInterval = 12

//This is the name used on the backend for the pixiv service, any api request accessing this service must match this string to identify the service
const pixivServiceName = "pixiv"

const PixivDelaySec = 1.0 //seconds between requests
const PixivQpT = 1        //number of queries between delays
