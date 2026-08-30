package main

import "testing"

func TestFormFor(t *testing.T) {
	if got := formFor("poem", []string{"床前明月光，疑是地上霜。", "举头望明月，低头思故乡。"}); got != "五言绝句" {
		t.Fatalf("unexpected form %s", got)
	}
	if got := formFor("poem", []string{"白日依山尽", "黄河入海流", "欲穷千里目", "更上一层楼"}); got != "五言绝句" {
		t.Fatalf("unexpected form %s", got)
	}
}
func TestNormalizeTraditional(t *testing.T) {
	if normalize("嶽 風，萬里") != "岳风万里" {
		t.Fatalf("normalization failed")
	}
}

func TestFamousScoreUsesLineMarkerForSharedCipai(t *testing.T) {
	if got := famousScore("苏轼|水调歌头", "安石在东海，从事鬓惊秋"); got != 0 {
		t.Fatalf("unrelated work must not be featured, got %d", got)
	}
	if got := famousScore("苏轼|水调歌头·明月几时有", "明月几时有，把酒问青天"); got != 98 {
		t.Fatalf("expected canonical work score, got %d", got)
	}
}

func TestTuneFor(t *testing.T) {
	if got := tuneFor("ci", "水调歌头·明月几时有"); got != "水调歌头" {
		t.Fatalf("unexpected ci tune %q", got)
	}
	if got := tuneFor("qu", "双调·水仙子·夜雨"); got != "双调·水仙子" {
		t.Fatalf("unexpected qu tune %q", got)
	}
	if got := tuneFor("ci", "浣溪沙（其一）"); got != "浣溪沙" {
		t.Fatalf("unexpected parenthesized tune %q", got)
	}
}
