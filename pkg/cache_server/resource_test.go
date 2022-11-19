package cache_server

import "testing"

/*
Because this code is called at a high frequency, we have to do some benchmark for them
*/

func BenchmarkParseReadResource(b *testing.B) {
	// [<instance>/]blobs/<hash>/<size>[/<filename>]
	// case1 instance/blobs/23d93e6e6ef656661d36b2afd301d277692ded016abe558650b4c813c7c369cf/141/filename
	for i := 0; i < b.N; i++ {
		_, _ = ParseReadResource("instance/blobs/23d93e6e6ef656661d36b2afd301d277692ded016abe558650b4c813c7c369cf/141/filename")
	}
}
