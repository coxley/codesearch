package main

import (
	"strings"
	"testing"

	"golang.org/x/exp/slices"
)

// var searchResults = SearchResult{
// 	FileKey{Owner: "TriggerMail", Name: "luci-go", Path: "logdog/common/storage/storage.go"}: []TextMatch{
// 		{
// 			Fragment: " and goroutine-safe.\n//\n// All methods may return errors.Transient errors if they encounter an error",
// 			Indices:  [][2]int{{82, 91}},
// 		},
// 	},
// }
//
// var defaultBranches = map[string]string{
// 	"TriggerMail/luci-go": "master",
// }
//
// var fullText = FullText{
// 	Values: map[FileKey]string{
// 		{Owner: "TriggerMail", Name: "luci-go", Path: "logdog/common/storage/storage.go"}: "// Copyright 2016 The LUCI Authors.\n//\n// Licensed under the Apache License, Version 2.0 (the \"License\");\n// you may not use this file except in compliance with the License.\n// You may obtain a copy of the License at\n//\n//      http://www.apache.org/licenses/LICENSE-2.0\n//\n// Unless required by applicable law or agreed to in writing, software\n// distributed under the License is distributed on an \"AS IS\" BASIS,\n// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.\n// See the License for the specific language governing permissions and\n// limitations under the License.\n\npackage datastorecache\n\nimport (\n\t\"bytes\"\n\t\"context\"\n\t\"fmt\"\n\t\"net/http\"\n\t\"strconv\"\n\t\"testing\"\n\t\"time\"\n\n\t\"github.com/TriggerMail/luci-go/common/errors\"\n\n\t\"go.chromium.org/gae/service/datastore\"\n\n\t. \"github.com/smartystreets/goconvey/convey\"\n)\n\nfunc testManagerImpl(t *testing.T, mgrTemplate *manager) {\n\tt.Parallel()\n\n\tConvey(`A testing manager setup`, t, withTestEnv(func(te *testEnv) {\n\t\t// Build our Manager, using template parameters.\n\t\tcache := makeTestCache(\"test-cache\")\n\t\tmgr := cache.manager()\n\t\tif mgrTemplate != nil {\n\t\t\tmgr.queryBatchSize = mgrTemplate.queryBatchSize\n\t\t}\n\n\t\tconst cronPath = \"/cron/manager\"\n\t\tmgr.installCronRoute(cronPath, te.Router, te.Middleware)\n\n\t\trunCron := func() int {\n\t\t\tdatastore.GetTestable(te).CatchupIndexes()\n\t\t\tresp, err := http.Get(te.Server.URL + cronPath)\n\t\t\tif err != nil {\n\t\t\t\tpanic(fmt.Errorf(\"failed to GET: %s\", err))\n\t\t\t}\n\t\t\treturn resp.StatusCode\n\t\t}\n\n\t\tstatsForShard := func(id int) *managerShardStats {\n\t\t\tst := managerShardStats{\n\t\t\t\tShard: id + 1,\n\t\t\t}\n\t\t\tSo(datastore.Get(cache.withNamespace(te), &st), ShouldBeNil)\n\t\t\treturn &st\n\t\t}\n\n\t\t// Useful times.\n\t\tvar (\n\t\t\tnow                = te.Clock.Now()\n\t\t\taccessedRecently   = now.Add(-time.Second)\n\t\t\taccessedNeedsPrune = now.Add(-cache.pruneInterval())\n\n\t\t\trefreshUnnecessary = now.Add(-time.Second)\n\t\t\trefreshNeeded      = now.Add(-cache.refreshInterval)\n\t\t)\n\n\t\tConvey(`Can run on an empty data set.`, func() {\n\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\tShard:             1,\n\t\t\t\tLastSuccessfulRun: now,\n\t\t\t})\n\t\t})\n\n\t\tConvey(`Returns error code for cache update errors.`, func() {\n\t\t\tSo(datastore.Put(cache.withNamespace(te), &entry{\n\t\t\t\tCacheName:     cache.Name,\n\t\t\t\tKey:           []byte(\"foo\"),\n\t\t\t\tLastAccessed:  accessedRecently,\n\t\t\t\tLastRefreshed: refreshNeeded,\n\t\t\t}), ShouldBeNil)\n\t\t\tdatastore.GetTestable(te).CatchupIndexes()\n\n\t\t\t// No refresh function installed, so this will fail.\n\t\t\tSo(runCron(), ShouldEqual, http.StatusInternalServerError)\n\t\t})\n\n\t\tConvey(`With cache entries installed`, func() {\n\t\t\tvar entries []*entry\n\t\t\tfor i, ce := range []struct {\n\t\t\t\tkey           string\n\t\t\t\tlastAccessed  time.Time\n\t\t\t\tlastRefreshed time.Time\n\t\t\t}{\n\t\t\t\t{\"idle\", accessedRecently, refreshUnnecessary},\n\t\t\t\t{\"refreshMe\", accessedRecently, refreshNeeded},\n\t\t\t\t{\"pruneMe\", accessedNeedsPrune, refreshUnnecessary},\n\t\t\t\t{\"pruneMeInsteeadOfRefresh\", accessedNeedsPrune, refreshNeeded},\n\t\t\t} {\n\t\t\t\tentries = append(entries, &entry{\n\t\t\t\t\tCacheName:     cache.Name,\n\t\t\t\t\tKey:           []byte(ce.key),\n\t\t\t\t\tLastAccessed:  ce.lastAccessed,\n\t\t\t\t\tLastRefreshed: ce.lastRefreshed,\n\t\t\t\t\tData:          []byte(ce.key),\n\t\t\t\t\tSchema:        \"test\",\n\t\t\t\t\tDescription:   fmt.Sprintf(\"test entry #%d\", i),\n\t\t\t\t})\n\t\t\t}\n\t\t\tSo(datastore.Put(cache.withNamespace(te), entries), ShouldBeNil)\n\n\t\t\tConvey(`Will refresh/prune the entries.`, func() {\n\t\t\t\tcache.refreshFn = func(context.Context, []byte, Value) (Value, error) {\n\t\t\t\t\treturn Value{\n\t\t\t\t\t\tSchema:      \"new schema\",\n\t\t\t\t\t\tDescription: \"new hotness\",\n\t\t\t\t\t\tData:        []byte(\"REFRESHED\"),\n\t\t\t\t\t}, nil\n\t\t\t\t}\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\tLastEntryCount:    4,\n\t\t\t\t})\n\t\t\t\tSo(cache.refreshes, ShouldEqual, 1)\n\n\t\t\t\t// All \"refreshMe\" should have their data updated.\n\t\t\t\tfor _, e := range entries {\n\t\t\t\t\tif !bytes.Equal(e.Key, []byte(\"refreshMe\")) {\n\t\t\t\t\t\tcontinue\n\t\t\t\t\t}\n\t\t\t\t\tSo(datastore.Get(cache.withNamespace(te), e), ShouldBeNil)\n\t\t\t\t\tSo(e.Data, ShouldResemble, []byte(\"REFRESHED\"))\n\t\t\t\t}\n\n\t\t\t\tConvey(`A second run will not refresh any entries.`, func() {\n\t\t\t\t\tcache.refreshFn = nil\n\t\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\t\tShard:             1,\n\t\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\t\tLastEntryCount:    2, // Pruned half of them.\n\t\t\t\t\t})\n\t\t\t\t})\n\t\t\t})\n\n\t\t\tConvey(`Will not prune any entries if AccessUpdateInterval is 0.`, func() {\n\t\t\t\tcache.refreshFn = func(c context.Context, key []byte, v Value) (Value, error) { return v, nil }\n\t\t\t\tcache.AccessUpdateInterval = 0\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\tLastEntryCount:    4,\n\t\t\t\t})\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\tLastEntryCount:    4, // None pruned.\n\t\t\t\t})\n\t\t\t})\n\n\t\t\tConvey(`Will not prune any entries if PruneFactor is 0.`, func() {\n\t\t\t\tcache.refreshFn = func(c context.Context, key []byte, v Value) (Value, error) { return v, nil }\n\t\t\t\tcache.PruneFactor = 0\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\tLastEntryCount:    4,\n\t\t\t\t})\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\tLastEntryCount:    4, // None pruned.\n\t\t\t\t})\n\t\t\t})\n\n\t\t\tConvey(`Will error if datastore PutMulti is broken.`, func() {\n\t\t\t\tte.DatastoreFB.BreakFeatures(errors.New(\"test error\"), \"PutMulti\")\n\n\t\t\t\tcache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, nil }\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusInternalServerError)\n\t\t\t\tSo(cache.refreshes, ShouldEqual, 1)\n\t\t\t})\n\n\t\t\tConvey(`Will error if datastore DeleteMulti is broken.`, func() {\n\t\t\t\tte.DatastoreFB.BreakFeatures(errors.New(\"test error\"), \"DeleteMulti\")\n\n\t\t\t\tcache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, nil }\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusInternalServerError)\n\t\t\t\tSo(cache.refreshes, ShouldEqual, 1)\n\t\t\t})\n\n\t\t\tConvey(`Will not refresh entries if there is an error.`, func() {\n\t\t\t\t// (The error is that we are refreshing, but there is no refresh\n\t\t\t\t// handler installed).\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusInternalServerError)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: time.Time{},\n\t\t\t\t\tLastEntryCount:    4,\n\t\t\t\t})\n\t\t\t})\n\n\t\t\tConvey(`Will delete cache entries, if requested.`, func() {\n\t\t\t\tcache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, ErrDeleteCacheEntry }\n\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\tLastEntryCount:    4,\n\t\t\t\t})\n\t\t\t\t// One of the four should have been refreshed, and requeted deletion.\n\t\t\t\tSo(cache.refreshes, ShouldEqual, 1)\n\n\t\t\t\t// Do a second run. Nothing should be refreshed, since all refresh\n\t\t\t\t// candidates were deleted.\n\t\t\t\tcache.reset()\n\t\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\t\tShard:             1,\n\t\t\t\t\tLastSuccessfulRun: now,\n\t\t\t\t\tLastEntryCount:    1, // Only \"idle\" remains.\n\t\t\t\t})\n\t\t\t\t// One of the four should have been refreshed, and requeted\n\t\t\t\t// deletion.\n\t\t\t\tSo(cache.refreshes, ShouldEqual, 0)\n\t\t\t})\n\t\t})\n\n\t\tConvey(`With cache entries, but no Handler function.`, func() {\n\t\t\tcache.HandlerFunc = nil\n\n\t\t\tvar entries []*entry\n\t\t\tfor i, la := range []time.Time{\n\t\t\t\taccessedRecently,\n\t\t\t\taccessedNeedsPrune,\n\t\t\t} {\n\t\t\t\tentries = append(entries, &entry{\n\t\t\t\t\tCacheName:     cache.Name,\n\t\t\t\t\tKey:           []byte(strconv.Itoa(i)),\n\t\t\t\t\tLastAccessed:  la,\n\t\t\t\t\tLastRefreshed: refreshNeeded,\n\t\t\t\t\tData:          nil,\n\t\t\t\t\tSchema:        \"test\",\n\t\t\t\t\tDescription:   fmt.Sprintf(\"test entry #%d\", i),\n\t\t\t\t})\n\t\t\t}\n\t\t\tSo(datastore.Put(cache.withNamespace(te), entries), ShouldBeNil)\n\n\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\tShard:             1,\n\t\t\t\tLastSuccessfulRun: te.Clock.Now(),\n\t\t\t\tLastEntryCount:    2,\n\t\t\t})\n\n\t\t\t// All are expired.\n\t\t\tte.Clock.Add(cache.pruneInterval())\n\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\tShard:             1,\n\t\t\t\tLastSuccessfulRun: te.Clock.Now(),\n\t\t\t\tLastEntryCount:    1,\n\t\t\t})\n\n\t\t\t// All have now been pruned.\n\t\t\tSo(runCron(), ShouldEqual, http.StatusOK)\n\t\t\tSo(statsForShard(0), ShouldResemble, &managerShardStats{\n\t\t\t\tShard:             1,\n\t\t\t\tLastSuccessfulRun: te.Clock.Now(),\n\t\t\t\tLastEntryCount:    0,\n\t\t\t})\n\t\t})\n\n\t\tConvey(`Will return an error if Run is broken during Handler query.`, func() {\n\t\t\tte.DatastoreFB.BreakFeatures(errors.New(\"test error\"), \"Run\")\n\t\t\tSo(runCron(), ShouldEqual, http.StatusInternalServerError)\n\t\t})\n\n\t\tConvey(`Will return an error if Run is broken during entry refresh.`, func() {\n\t\t\t// We will Put \"queryBatchSize+1\" expired entries, then execute our\n\t\t\t// handler. The first round will refresh, at which point our refresh\n\t\t\t// handler will break Run. The next round will encounter the broken Run\n\t\t\t// and\n\t\t\tentries := make([]*entry, mgr.queryBatchSize+1)\n\t\t\tfor i := range entries {\n\t\t\t\tentries[i] = &entry{\n\t\t\t\t\tCacheName:    cache.Name,\n\t\t\t\t\tKey:          []byte(fmt.Sprintf(\"entry-%d\", i)),\n\t\t\t\t\tLastAccessed: accessedRecently,\n\t\t\t\t}\n\t\t\t}\n\t\t\tSo(datastore.Put(cache.withNamespace(te), entries), ShouldBeNil)\n\n\t\t\tcache.refreshFn = func(context.Context, []byte, Value) (Value, error) {\n\t\t\t\tte.DatastoreFB.BreakFeatures(errors.New(\"test error\"), \"Run\")\n\t\t\t\treturn Value{}, nil\n\t\t\t}\n\n\t\t\tSo(runCron(), ShouldEqual, http.StatusInternalServerError)\n\t\t})\n\t}))\n}\n\n// TestManager runs the manager tests against the production manager instance.\n// This exercises the production settings.\nfunc TestManager(t *testing.T) {\n\ttestManagerImpl(t, nil)\n}\n\n// TestManagerConstrained runs the manager tests against a resource-constrained\n// manager instance. This exercises limited (but value) manager settings.\nfunc TestManagerConstrained(t *testing.T) {\n\ttestManagerImpl(t, &manager{\n\t\tqueryBatchSize: 1,\n\t})\n}\n",
// 	},
// 	Truncated: map[FileKey]bool{},
// }
//
// func TestInFull(t *testing.T) {
// 	key := FileKey{Owner: "TriggerMail", Name: "luci-go", Path: "logdog/common/storage/storage.go"}
// 	fmt.Printf("fullText: %s\n", fullText.Values[key])
// 	fmt.Printf("fragment: %s\n", searchResults[key][0].Fragment)
// 	t.Fail()
// }

func TestContext(t *testing.T) {
	type data struct {
		content  string
		lineno   int
		before   int
		after    int
		expected []string
	}

	td := []data{
		{"foo bar baz", 1, 5, 5, []string{}},
		{"foo bar baz\n", 1, 5, 5, []string{""}},
		{"\nfoo bar baz\n", 2, 5, 5, []string{"", ""}},

		{"foo\nbar\nbaz\n", 2, 1, 0, []string{"foo"}},
		{"foo\nbar\nbaz\n", 2, 2, 0, []string{"foo"}},
		{"foo\nbar\nbaz\n", 2, 3, 0, []string{"foo"}},

		{"foo\nbar\nbaz\n", 1, 1, 0, []string{}},
		{"foo\nbar\nbaz\n", 1, 2, 0, []string{}},
		{"foo\nbar\nbaz\n", 1, 3, 0, []string{}},
		{"foo\nbar\nbaz\n", 1, 0, 1, []string{"bar"}},
		{"foo\nbar\nbaz\n", 1, 0, 2, []string{"bar", "baz"}},
		{"foo\nbar\nbaz\n", 1, 0, 3, []string{"bar", "baz", ""}},

		{"foo\nbar\nbaz\n", 2, 1, 1, []string{"foo", "baz"}},
	}
	for _, test := range td {
		leading, trailing := contextLines(test.content, test.lineno, test.before, test.after)
		if slices.Compare(append(leading, trailing...), test.expected) != 0 {
			t.Errorf("expected: %+v, got: %+v", test.expected, append(leading, trailing...))
		}
	}
}

func BenchmarkContext(b *testing.B) {
	// >80 columns wide, 100k LOC
	testFile := strings.Repeat(strings.Repeat("foo bar baz", 8)+"\n", 100000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		contextLines(testFile, 5000, 5, 5)
	}

}
