package eventschema

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// The benchmarks in this file exercise enrichment against the released OCSF
// schema fixture with an event that is representative of production telemetry:
// a detection_finding carrying a device, a nested actor process chain, and a
// twenty element evidences array of process, file, user, and endpoint
// artifacts. The event generates hundreds of observables, dozens of enum
// siblings, and a large number of duplicate observable diagnostics, so it
// covers the traversal, observable, and diagnostic paths that dominate real
// event processing cost. The existing BenchmarkProcessEvent* benchmarks use a
// small hand-built schema and event, which keeps them precise but leaves those
// paths nearly unexercised.

const benchmarkEvidenceCount = 20

func makeRealSchema(assert *require.Assertions) *Schema {
	schema, _, err := Load(testSchemaFilePath)
	assert.NoError(err)
	return schema
}

// benchmarkDetectionFinding builds a detection_finding event with roughly six
// hundred attributes. Integral numbers use int64 because OCSF integer_t and long_t
// values are signed 64-bit integers and enum lookups require an integer kind.
func benchmarkDetectionFinding() jsonish.Map {
	return jsonish.Map{
		"activity_id":   int64(1),
		"category_uid":  int64(2),
		"class_uid":     int64(2004),
		"type_uid":      int64(200401),
		"severity_id":   int64(4),
		"status_id":     int64(1),
		"confidence_id": int64(3),
		"impact_id":     int64(3),
		"risk_level_id": int64(3),
		"time":          int64(1735689600000),
		"count":         int64(3),
		"message":       "Suspicious process chain detected on endpoint",
		"metadata":      benchmarkMetadata(),
		"device":        benchmarkDevice(),
		"actor":         benchmarkActor(),
		"finding_info":  benchmarkFindingInfo(),
		"evidences":     benchmarkEvidences(),
	}
}

func benchmarkMetadata() jsonish.Map {
	return jsonish.Map{
		"version":         "1.8.0",
		"correlation_uid": "5f9c1e2a-7b3d-4c8e-9a1f-2b3c4d5e6f70",
		"uid":             "b1f2c3d4-e5f6-4708-9a1b-2c3d4e5f6071",
		"log_name":        "edr-detections",
		"log_provider":    "SentinelSuite",
		"logged_time":     int64(1735689601000),
		"tenant_uid":      "tenant-88213",
		"event_code":      "EDR-4471",
		"product": jsonish.Map{
			"name":        "SentinelSuite EDR",
			"vendor_name": "Example Security",
			"version":     "3.4.1",
			"uid":         "product-1182",
		},
		"profiles": []string{"host", "security_control"},
		"labels":   []string{"production", "endpoint"},
	}
}

func benchmarkDevice() jsonish.Map {
	return jsonish.Map{
		"type_id":         int64(1),
		"uid":             "device-a1b2c3d4e5",
		"name":            "WIN-FIN-0042",
		"hostname":        "win-fin-0042.corp.example.com",
		"ip":              "10.42.13.201",
		"mac":             "00:1B:44:11:3A:B7",
		"domain":          "corp.example.com",
		"instance_uid":    "i-0abcd1234ef567890",
		"is_managed":      true,
		"risk_score":      int64(72),
		"first_seen_time": int64(1704067200000),
		"last_seen_time":  int64(1735689500000),
		"zone":            "corp-finance",
		"os": jsonish.Map{
			"name":    "Windows 11 Enterprise",
			"version": "10.0.22631",
			"build":   "22631.4460",
			"edition": "Enterprise",
		},
		"hw_info": jsonish.Map{
			"cpu_type":      "x86_64",
			"cpu_cores":     int64(8),
			"ram_size":      int64(32768),
			"serial_number": "SN-4471-0042",
			"vendor_name":   "Example Hardware",
		},
		"location": jsonish.Map{
			"city":        "Boston",
			"country":     "US",
			"region":      "MA",
			"coordinates": []float64{-71.0589, 42.3601},
		},
		"owner": benchmarkUser(),
	}
}

func benchmarkActor() jsonish.Map {
	return jsonish.Map{
		"process":    benchmarkProcessChain(),
		"user":       benchmarkUser(),
		"invoked_by": "taskeng.exe",
		"session": jsonish.Map{
			"uid":           "sess-77120",
			"uuid":          "6d5c4b3a-2f1e-4d0c-9b8a-7f6e5d4c3b2a",
			"created_time":  int64(1735689000000),
			"is_remote":     false,
			"logon_type_id": int64(2),
		},
	}
}

// benchmarkProcessChain builds a three level process ancestry, which is the
// shape endpoint detections normally report.
func benchmarkProcessChain() jsonish.Map {
	grandparent := benchmarkProcess("explorer.exe", 2104, "C:\\Windows\\explorer.exe",
		"3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d0e9f8a7b6c5d4e3f2a")
	parent := benchmarkProcess("powershell.exe", 4412,
		"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
		"a1b2c3d4e5f60718293a4b5c6d7e8f901a2b3c4d5e6f708192a3b4c5d6e7f809")
	parent["parent_process"] = grandparent
	child := benchmarkProcess("rundll32.exe", 7788, "C:\\Windows\\System32\\rundll32.exe",
		"9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b3a2f1e0d9c8b7a6f5e4d3c2b1a0f9e8d")
	child["parent_process"] = parent
	return child
}

func benchmarkProcess(name string, pid int64, path string, sha256 string) jsonish.Map {
	return jsonish.Map{
		"name":     name,
		"pid":      pid,
		"uid":      "proc-" + strconv.FormatInt(pid, 10),
		"cmd_line": path + " --run --hidden",
		"file":     benchmarkFile(name, path, sha256),
		"user":     benchmarkUser(),
	}
}

func benchmarkFile(name string, path string, sha256 string) jsonish.Map {
	return jsonish.Map{
		"name":          name,
		"path":          path,
		"parent_folder": "C:\\Windows\\System32",
		"size":          int64(114688),
		"uid":           "file-" + sha256[:12],
		"company_name":  "Example Corp",
		"hashes": []jsonish.Map{
			{"algorithm_id": int64(3), "value": sha256},
		},
	}
}

func benchmarkUser() jsonish.Map {
	return jsonish.Map{
		"name":       "dana.reed",
		"uid":        "S-1-5-21-1004",
		"email_addr": "dana.reed@example.com",
		"full_name":  "Dana Reed",
		"domain":     "corp.example.com",
	}
}

func benchmarkEndpoint(hostname string, ip string, port int64) jsonish.Map {
	return jsonish.Map{
		"hostname": hostname,
		"ip":       ip,
		"port":     port,
		"uid":      "ep-" + hostname,
	}
}

func benchmarkFindingInfo() jsonish.Map {
	return jsonish.Map{
		"uid":             "finding-2004-88213",
		"title":           "Suspicious PowerShell spawning rundll32",
		"desc":            "A PowerShell process spawned rundll32.exe with an unusual command line.",
		"created_time":    int64(1735689600000),
		"first_seen_time": int64(1735689600000),
		"types":           []string{"Behavioral", "Endpoint"},
		"src_url":         "https://console.example.com/findings/88213",
		"analytic": jsonish.Map{
			"name":    "PowerShell Spawning Signed Binary Proxy",
			"uid":     "analytic-T1218-011",
			"type_id": int64(3),
			"version": "4",
		},
		"attacks": []jsonish.Map{{
			"technique": jsonish.Map{"name": "Signed Binary Proxy Execution", "uid": "T1218"},
			"version":   "14",
		}},
	}
}

// benchmarkEvidences builds the evidences array. Most artifact values repeat
// across elements, as they do when a detection reports many observations of the
// same file and destination, so the array exercises both observable generation
// and the duplicate observable diagnostics that repeated content produces.
func benchmarkEvidences() []jsonish.Map {
	evidences := make([]jsonish.Map, 0, benchmarkEvidenceCount)
	for index := range benchmarkEvidenceCount {
		suffix := strconv.Itoa(index)
		evidences = append(evidences, jsonish.Map{
			"name":       "artifact-" + suffix,
			"uid":        "evidence-" + suffix,
			"verdict_id": int64(2),
			"process": jsonish.Map{
				"name":     "rundll32.exe",
				"pid":      int64(7788 + index),
				"uid":      "proc-" + suffix,
				"cmd_line": "C:\\Windows\\System32\\rundll32.exe --run --hidden",
				"file": jsonish.Map{
					"name": "rundll32.exe",
					"path": "C:\\Windows\\System32\\rundll32.exe",
					"uid":  "file-9e8d7c6b5a4f",
				},
			},
			// Each evidence element reports a distinct dropped file but shares the
			// destination and principal, which is the usual mix of unique and
			// repeated artifact values in a detection.
			"file": jsonish.Map{
				"name":          "loader-" + suffix + ".dll",
				"path":          "C:\\Users\\dana.reed\\AppData\\Local\\Temp\\loader-" + suffix + ".dll",
				"parent_folder": "C:\\Users\\dana.reed\\AppData\\Local\\Temp",
				"uid":           "file-1a2b3c4d5e" + suffix,
				"hashes": []jsonish.Map{
					{
						"algorithm_id": int64(3),
						"value":        "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e" + suffix,
					},
				},
			},
			"user": jsonish.Map{
				"name": "dana.reed", "uid": "S-1-5-21-1004", "email_addr": "dana.reed@example.com",
			},
			"src_endpoint": benchmarkEndpoint("win-fin-0042.corp.example.com", "10.42.13.201", int64(49152+index)),
			"dst_endpoint": benchmarkEndpoint("cdn.example-malicious.test", "203.0.113.77", 443),
		})
	}
	return evidences
}

// resetBenchmarkEvent removes the attributes enrichment adds so that each
// iteration starts from the same unenriched event.
func resetBenchmarkEvent(event jsonish.Map) {
	delete(event, "observables")
	for _, sibling := range benchmarkTopLevelSiblings {
		delete(event, sibling)
	}
	device, deviceOK := event["device"].(jsonish.Map)
	actor, actorOK := event["actor"].(jsonish.Map)
	session, sessionOK := actor["session"].(jsonish.Map)
	findingInfo, findingInfoOK := event["finding_info"].(jsonish.Map)
	analytic, analyticOK := findingInfo["analytic"].(jsonish.Map)
	evidences, evidencesOK := event["evidences"].([]jsonish.Map)
	if !deviceOK || !actorOK || !sessionOK || !findingInfoOK || !analyticOK || !evidencesOK {
		panic("representative benchmark event has an unexpected shape")
	}
	delete(device, "type")
	delete(session, "logon_type")
	delete(analytic, "type")
	for _, evidence := range evidences {
		delete(evidence, "verdict")
		file, fileOK := evidence["file"].(jsonish.Map)
		hashes, hashesOK := file["hashes"].([]jsonish.Map)
		if !fileOK || !hashesOK || len(hashes) == 0 {
			panic("representative benchmark evidence has an unexpected shape")
		}
		delete(hashes[0], "algorithm")
	}
	for _, hashes := range benchmarkProcessChainHashes(event) {
		delete(hashes, "algorithm")
	}
}

// benchmarkProcessChainHashes returns the file hash objects in the actor
// process ancestry, which receive enum siblings during enrichment.
func benchmarkProcessChainHashes(event jsonish.Map) []jsonish.Map {
	var hashes []jsonish.Map
	process, ok := event["actor"].(jsonish.Map)["process"].(jsonish.Map)
	for ok {
		if file, present := process["file"].(jsonish.Map); present {
			if entries, present := file["hashes"].([]jsonish.Map); present {
				hashes = append(hashes, entries...)
			}
		}
		process, ok = process["parent_process"].(jsonish.Map)
	}
	return hashes
}

var benchmarkTopLevelSiblings = []string{
	"activity_name", "category_name", "class_name", "type_name", "severity",
	"status", "confidence", "impact", "risk_level",
}

func BenchmarkProcessEventEnrichmentDetectionFinding(b *testing.B) {
	assert := require.New(b)
	schema := makeRealSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	event := benchmarkDetectionFinding()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resetBenchmarkEvent(event)
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventEnrichmentDetectionFindingParallel(b *testing.B) {
	assert := require.New(b)
	schema := makeRealSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine owns an event because enrichment mutates events in place.
		event := benchmarkDetectionFinding()
		for pb.Next() {
			resetBenchmarkEvent(event)
			if _, err := pipeline.ProcessEvent(event); err != nil {
				b.Fatal(err)
			}
		}
	})
}
