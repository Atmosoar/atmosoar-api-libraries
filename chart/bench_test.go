package chart

import "testing"

// The three encodings share one resolve-and-layout pass, so these numbers
// separate the derivation cost from each renderer's transcription cost.
func BenchmarkResolve(b *testing.B) {
	spec := specFixtures()["meteogram-light"]
	for b.Loop() {
		if _, err := spec.Resolved(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuild(b *testing.B) {
	spec := specFixtures()["meteogram-light"]
	for b.Loop() {
		if _, err := Render(spec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSVG(b *testing.B) {
	spec := specFixtures()["meteogram-light"]
	b.ReportAllocs()
	for b.Loop() {
		if _, err := SVG(spec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPNG(b *testing.B) {
	spec := specFixtures()["meteogram-light"]
	b.ReportAllocs()
	for b.Loop() {
		if _, err := PNG(spec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWindrosePNG(b *testing.B) {
	spec := specFixtures()["windrose-light"]
	b.ReportAllocs()
	for b.Loop() {
		if _, err := PNG(spec); err != nil {
			b.Fatal(err)
		}
	}
}
