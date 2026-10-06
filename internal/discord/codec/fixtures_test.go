package codec_test

// The fixtures are written by hand from Discord's RPC documentation and the
// protocol notes of its old library, checked on 2026-10-03. They are not
// produced by the codec. Each is a header of two little-endian 32-bit
// integers, the opcode and the payload length, and then the JSON.

const (
	fixtureNonce = "647d814a-4cf8-4fbb-948f-898abd24f55b"

	// Opcode 0, length 40.
	fixtureHandshake = "\x00\x00\x00\x00\x28\x00\x00\x00" +
		`{"v":1,"client_id":"123456789012345678"}`

	// Opcode 1, length 309. The documentation's example, without the fields
	// this project does not send.
	fixtureSetActivity = "\x01\x00\x00\x00\x35\x01\x00\x00" +
		`{"cmd":"SET_ACTIVITY","args":{"pid":9999,"activity":{` +
		`"state":"In a Group","details":"Competitive | In a Match",` +
		`"timestamps":{"start":1507665886},` +
		`"assets":{"large_image":"numbani_map","large_text":"Numbani",` +
		`"small_image":"pharah_profile","small_text":"Pharah"}}},` +
		`"nonce":"647d814a-4cf8-4fbb-948f-898abd24f55b"}`

	// Opcode 1, length 386. The same activity with one button, in the shape of
	// the buttons field of Discord's activity object, checked on 2026-10-05.
	fixtureSetActivityButton = "\x01\x00\x00\x00\x82\x01\x00\x00" +
		`{"cmd":"SET_ACTIVITY","args":{"pid":9999,"activity":{` +
		`"state":"In a Group","details":"Competitive | In a Match",` +
		`"timestamps":{"start":1507665886},` +
		`"assets":{"large_image":"numbani_map","large_text":"Numbani",` +
		`"small_image":"pharah_profile","small_text":"Pharah"},` +
		`"buttons":[{"label":"View on GitHub","url":"https://github.com/me/visions"}]}},` +
		`"nonce":"647d814a-4cf8-4fbb-948f-898abd24f55b"}`

	// Opcode 1, length 105.
	fixtureClearActivity = "\x01\x00\x00\x00\x69\x00\x00\x00" +
		`{"cmd":"SET_ACTIVITY","args":{"pid":9999,"activity":null},` +
		`"nonce":"647d814a-4cf8-4fbb-948f-898abd24f55b"}`

	fixtureReady = `{"cmd":"DISPATCH","data":{"v":1,"config":{"cdn_host":"cdn.discordapp.com",` +
		`"api_endpoint":"//discord.com/api","environment":"production"},` +
		`"user":{"id":"53908232506183680","username":"Mason","discriminator":"1337","avatar":null}},` +
		`"evt":"READY","nonce":null}`

	fixtureError = `{"cmd":"SET_ACTIVITY","data":{"code":4000,"message":"Invalid payload"},` +
		`"evt":"ERROR","nonce":"647d814a-4cf8-4fbb-948f-898abd24f55b"}`

	fixtureAck = `{"cmd":"SET_ACTIVITY","data":{"state":"In a Group","application_id":"123456789012345678"},` +
		`"evt":null,"nonce":"647d814a-4cf8-4fbb-948f-898abd24f55b"}`

	fixtureClose = `{"code":4000,"message":"Invalid Client ID"}`

	fixturePing = `{"ping":1}`
)
