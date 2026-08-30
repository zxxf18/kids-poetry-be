package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/zxxf18/kids-poetry-be/internal/model"
)

type sourceRecord struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	AuthorName   string   `json:"authorName"`
	Dynasty      string   `json:"dynasty"`
	Content      []string `json:"content"`
	Pinyin       []string `json:"pinyin"`
	Translation  string   `json:"translation"`
	Annotation   []string `json:"annotation"`
	Appreciation string   `json:"appreciation"`
}
type selectedPoem struct {
	Author     string   `json:"author"`
	Title      string   `json:"title"`
	Rhythmic   string   `json:"rhythmic"`
	Paragraphs []string `json:"paragraphs"`
	Tags       []string `json:"tags"`
}

type sourceFile struct {
	Path string
	Kind string
}

type qualityStats struct {
	SourceRecords        int
	SkippedInvalid       int
	Total                int
	WithPinyin           int
	WithTranslation      int
	WithAnnotations      int
	WithAppreciation     int
	CompleteLearningData int
}

var famous = map[string]int{
	"李白|静夜思": 100, "王之涣|登鹳雀楼": 99, "李白|望庐山瀑布": 98, "李白|早发白帝城": 97,
	"李白|赠汪伦": 96, "李白|黄鹤楼送孟浩然之广陵": 95, "杜甫|望岳": 94, "杜甫|春夜喜雨": 93,
	"杜甫|闻官军收河南河北": 92, "杜甫|江南逢李龟年": 91, "杜甫|登高": 90, "张继|枫桥夜泊": 89,
	"孟郊|游子吟": 88,
}
var primarySchool = map[string]bool{"李白|静夜思": true, "王之涣|登鹳雀楼": true, "李白|望庐山瀑布": true, "李白|早发白帝城": true, "李白|赠汪伦": true, "李白|黄鹤楼送孟浩然之广陵": true, "杜甫|春夜喜雨": true, "杜甫|江南逢李龟年": true, "孟郊|游子吟": true, "张继|枫桥夜泊": true}
var pinyinCorrections = map[string][]string{
	"adcfe4db0c7f747783ceee65775aa7c8": {"jì lì dāng jì tiān xià lì", "qiú míng yīng qiú wàn shì míng"},
}

func main() {
	poetryRoot := flag.String("poetry-source", "", "poetry-source checkout")
	chineseRoot := flag.String("chinese-poetry", "", "chinese-poetry checkout")
	outDir := flag.String("out", "", "output directory")
	version := flag.String("version", time.Now().Format("2006-01-02")+".v1", "dataset version")
	poetryCommit := flag.String("poetry-commit", "815fd2d12d231a1ad47dfe422e942e069264b2a2", "poetry-source commit")
	chineseCommit := flag.String("chinese-commit", "b8594f81a89752241442f2ce267d6f66f96704ee", "chinese-poetry commit")
	flag.Parse()
	if *poetryRoot == "" || *chineseRoot == "" || *outDir == "" {
		fatal(fmt.Errorf("-poetry-source, -chinese-poetry and -out are required"))
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	selectedByContent, selectedByTitle, cipaiByContent := loadChineseMetadata(*chineseRoot)
	files := discoverSourceFiles(*poetryRoot)
	if len(files) == 0 {
		fatal(fmt.Errorf("no poetry-source shards found under %s", *poetryRoot))
	}
	dataPath := filepath.Join(*outDir, "poems.jsonl.gz")
	stats, err := writeDataset(dataPath, files, selectedByContent, selectedByTitle, cipaiByContent, *poetryCommit)
	if err != nil {
		fatal(err)
	}
	digest := fileSHA(dataPath)
	manifest := model.DatasetManifest{Version: *version, GeneratedAt: time.Now().UTC(), Count: stats.Total, SHA256: digest, Sources: map[string]string{"snowtraces/poetry-source": *poetryCommit, "chinese-poetry/chinese-poetry": *chineseCommit}, Selection: "snowtraces/poetry-source 中诗、词、曲的全部分片记录；正文和行级拼音做全量结构校验，仅跳过无法构成可阅读作品的空正文记录；译文、注释与赏析按源数据实际覆盖保留；精选集、词牌与知名度信号由 chinese-poetry 和人工名单补充。", Quality: map[string]int{"sourceRecords": stats.SourceRecords, "skippedInvalid": stats.SkippedInvalid, "withPinyin": stats.WithPinyin, "withTranslation": stats.WithTranslation, "withAnnotations": stats.WithAnnotations, "withAppreciation": stats.WithAppreciation, "completeLearningData": stats.CompleteLearningData}}
	writeJSON(filepath.Join(*outDir, "manifest.json"), manifest)
	attribution := `# 数据来源与使用说明

- 主记录与拼音：snowtraces/poetry-source，固定提交 ` + *poetryCommit + `
- 精选集、词牌与知名度辅助信息：chinese-poetry/chinese-poetry，固定提交 ` + *chineseCommit + `

收录范围为 poetry-source 诗、词、曲的全量分片；源仓唯一整首缺失的两行拼音记录由本项目人工补齐并在整理代码中保留固定修正。多数古代诗词原文已进入公共领域，但源仓也可能含近现代作品；仓库中的现代译文、注释、赏析等附加文本不因代码许可证而自动获得商业授权。本数据包保留字段级来源，当前用于个人学习型站点；对外商业发布前应按作品和附加文本继续完成来源审核，或替换为自有人工整理版本。
`
	if err := os.WriteFile(filepath.Join(*outDir, "ATTRIBUTION.md"), []byte(attribution), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("version=%s source=%d records=%d skipped=%d pinyin=%d translation=%d annotations=%d complete=%d sha256=%s out=%s\n", *version, stats.SourceRecords, stats.Total, stats.SkippedInvalid, stats.WithPinyin, stats.WithTranslation, stats.WithAnnotations, stats.CompleteLearningData, digest, *outDir)
}

func discoverSourceFiles(root string) []sourceFile {
	definitions := []struct {
		Dir, Prefix, Kind string
	}{
		{Dir: "诗", Prefix: "poetry", Kind: "poem"},
		{Dir: "词", Prefix: "ci", Kind: "ci"},
		{Dir: "曲", Prefix: "qu", Kind: "qu"},
	}
	files := make([]sourceFile, 0, 600)
	for _, definition := range definitions {
		pattern := filepath.Join(root, "source", definition.Dir, "*", definition.Prefix+".*.[0-9][0-9][0-9][0-9].json")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fatal(err)
		}
		for _, path := range matches {
			files = append(files, sourceFile{Path: path, Kind: definition.Kind})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func loadSource(path string) []sourceRecord { var v []sourceRecord; decode(path, &v); return v }
func loadPinyin(path string) map[string][]string {
	var v []sourceRecord
	decode(path, &v)
	m := map[string][]string{}
	for _, r := range v {
		m[r.ID] = r.Pinyin
	}
	return m
}
func decode(path string, v any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal(err)
	}
	if err = json.Unmarshal(data, v); err != nil {
		fatal(fmt.Errorf("decode %s: %w", path, err))
	}
}

func loadChineseMetadata(root string) (map[string][]string, map[string][]string, map[string]string) {
	byContent := map[string][]string{}
	byTitle := map[string][]string{}
	cipai := map[string]string{}
	for _, rel := range []string{filepath.Join("全唐诗", "唐诗三百首.json"), filepath.Join("宋词", "宋词三百首.json")} {
		var items []selectedPoem
		decode(filepath.Join(root, rel), &items)
		for _, p := range items {
			key := contentKey(p.Author, p.Paragraphs)
			byContent[key] = unique(append(byContent[key], p.Tags...))
			title := p.Title
			if title == "" {
				title = p.Rhythmic
			}
			byTitle[titleKey(p.Author, title)] = unique(append(byTitle[titleKey(p.Author, title)], p.Tags...))
			if p.Rhythmic != "" {
				cipai[key] = p.Rhythmic
			}
		}
	}
	files, _ := filepath.Glob(filepath.Join(root, "宋词", "ci.song.*.json"))
	for _, path := range files {
		var items []selectedPoem
		decode(path, &items)
		for _, p := range items {
			if p.Rhythmic != "" {
				cipai[contentKey(p.Author, p.Paragraphs)] = p.Rhythmic
			}
		}
	}
	return byContent, byTitle, cipai
}

func formFor(kind string, lines []string) string {
	if kind == "ci" {
		return "词"
	}
	if kind == "qu" {
		return "曲"
	}
	parts := clauses(lines)
	if len(parts) == 0 {
		return "诗"
	}
	same := true
	count := hanCount(parts[0])
	for _, line := range parts[1:] {
		if hanCount(line) != count {
			same = false
		}
	}
	if same && len(parts) == 4 && count == 5 {
		return "五言绝句"
	}
	if same && len(parts) == 4 && count == 7 {
		return "七言绝句"
	}
	if same && len(parts) == 8 && count == 5 {
		return "五言律诗"
	}
	if same && len(parts) == 8 && count == 7 {
		return "七言律诗"
	}
	if strings.Contains(strings.Join(lines, ""), "兮") {
		return "古风"
	}
	return "古诗"
}
func clauses(lines []string) []string {
	out := []string{}
	for _, line := range lines {
		for _, part := range strings.FieldsFunc(line, func(r rune) bool { return strings.ContainsRune("，。！？；!?;", r) }) {
			if strings.TrimSpace(part) != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
func hanCount(s string) int {
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n
}

func themesFor(title, content string, tags []string) []string {
	all := title + content + strings.Join(tags, "")
	groups := []struct {
		name string
		keys []string
	}{{"思乡", []string{"乡", "故园", "归家", "客愁", "乡思"}}, {"山水", []string{"山", "江", "河", "湖", "瀑布", "峰", "溪"}}, {"四季", []string{"春", "夏", "秋", "冬", "雪", "梅", "荷"}}, {"友情", []string{"友", "送别", "赠", "别君", "相逢"}}, {"边塞", []string{"塞", "关山", "胡马", "戍", "征人"}}, {"月夜", []string{"月", "夜", "星", "月明"}}, {"田园", []string{"田", "村", "农", "柴门", "牧"}}, {"咏物", []string{"咏物", "花", "竹", "柳", "鸟"}}, {"家国", []string{"国", "战", "军", "长安", "故国"}}}
	out := []string{}
	for _, g := range groups {
		for _, k := range g.keys {
			if strings.Contains(all, k) {
				out = append(out, g.name)
				break
			}
		}
	}
	if len(out) == 0 {
		out = []string{"人生感怀"}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}
func ageFor(lines, notes []string) (int, int) {
	chars := 0
	for _, line := range lines {
		chars += hanCount(line)
	}
	if chars <= 28 && len(notes) <= 8 {
		return 6, 9
	}
	if chars <= 64 {
		return 8, 11
	}
	return 10, 14
}
func collectionsFor(author, title, content string, tags []string) ([]string, int) {
	out := []string{}
	key := author + "|" + title
	score := famousScore(key, content)
	joined := strings.Join(tags, ",")
	if score > 0 {
		out = append(out, "widely-known")
	}
	if primarySchool[key] {
		out = append(out, "primary-school")
		if score < 85 {
			score = 85
		}
	}
	if strings.Contains(joined, "唐诗三百首") || strings.Contains(joined, "宋词三百首") {
		out = append(out, "classic-anthology")
		if score < 70 {
			score = 70
		}
	}
	if score >= 90 {
		out = append(out, "first-poems")
	}
	return unique(out), score
}

func famousScore(key, content string) int {
	if score := famous[key]; score > 0 {
		return score
	}
	markers := []struct {
		key, line string
		score     int
	}{
		{"苏轼|水调歌头", "明月几时有", 98},
		{"苏轼|水调歌头·明月几时有", "明月几时有", 98},
		{"苏轼|念奴娇", "大江东去", 96},
		{"李清照|声声慢", "寻寻觅觅", 95},
		{"李清照|如梦令", "昨夜雨疏风骤", 94},
		{"李清照|如梦令", "常记溪亭日暮", 93},
		{"李清照|武陵春", "风住尘香花已尽", 92},
		{"辛弃疾|青玉案", "东风夜放花千树", 89},
		{"岳飞|满江红", "怒发冲冠", 88},
	}
	for _, marker := range markers {
		if key == marker.key && strings.Contains(content, marker.line) {
			return marker.score
		}
	}
	return 0
}

func contentKey(author string, lines []string) string {
	return normalize(author) + "|" + normalize(strings.Join(lines, ""))
}
func titleKey(author, title string) string { return normalize(author) + "|" + normalize(title) }
func normalize(s string) string {
	replacer := strings.NewReplacer("臺", "台", "爲", "为", "萬", "万", "與", "与", "風", "风", "雲", "云", "國", "国", "歸", "归", "聲", "声", "見", "见", "後", "后", "裏", "里", "處", "处", "長", "长", "樓", "楼", "開", "开", "門", "门", "東", "东", "獨", "独", "無", "无", "來", "来", "時", "时", "還", "还", "頭", "头", "書", "书", "鄉", "乡", "嶽", "岳", "蟬", "蝉", "沈", "沉")
	s = replacer.Replace(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return -1
		}
		return r
	}, s)
}
func contentHash(author, title string, lines []string) string {
	sum := sha256.Sum256([]byte(normalize(author) + "|" + normalize(title) + "|" + normalize(strings.Join(lines, ""))))
	return hex.EncodeToString(sum[:])
}
func unique(v []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
func writeDataset(path string, files []sourceFile, selectedByContent, selectedByTitle map[string][]string, cipaiByContent map[string]string, poetryCommit string) (qualityStats, error) {
	stats := qualityStats{}
	temp, err := os.CreateTemp(filepath.Dir(path), ".poems-*.jsonl.gz")
	if err != nil {
		return stats, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	gz := gzip.NewWriter(temp)
	enc := json.NewEncoder(gz)
	enc.SetEscapeHTML(false)
	seenIDs := make(map[string]struct{}, 540000)
	for _, file := range files {
		records := loadSource(file.Path)
		pinyin := loadPinyin(strings.TrimSuffix(file.Path, ".json") + ".pinyin.json")
		for _, r := range records {
			stats.SourceRecords++
			if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.AuthorName) == "" || strings.TrimSpace(r.Dynasty) == "" || len(r.Content) == 0 {
				stats.SkippedInvalid++
				continue
			}
			if strings.TrimSpace(r.Title) == "" {
				r.Title = "无题"
			}
			if _, exists := seenIDs[r.ID]; exists {
				return stats, fmt.Errorf("duplicate source id %s", r.ID)
			}
			seenIDs[r.ID] = struct{}{}
			r.Pinyin = pinyin[r.ID]
			if correction, ok := pinyinCorrections[r.ID]; ok {
				r.Pinyin = correction
			}
			if len(r.Pinyin) != len(r.Content) {
				return stats, fmt.Errorf("content and pinyin lines do not align in %s for %s: %d != %d", file.Path, r.ID, len(r.Content), len(r.Pinyin))
			}
			key := contentKey(r.AuthorName, r.Content)
			tags := append([]string{}, selectedByContent[key]...)
			tags = append(tags, selectedByTitle[titleKey(r.AuthorName, r.Title)]...)
			cipai := ""
			if file.Kind == "ci" || file.Kind == "qu" {
				cipai = cipaiByContent[key]
				if cipai == "" {
					cipai = tuneFor(file.Kind, r.Title)
				}
			}
			content := strings.Join(r.Content, "")
			collections, score := collectionsFor(r.AuthorName, r.Title, content, tags)
			themes := themesFor(r.Title, content, tags)
			ageMin, ageMax := ageFor(r.Content, r.Annotation)
			item := model.PoemPayload{ID: r.ID, Title: strings.TrimSpace(r.Title), Author: strings.TrimSpace(r.AuthorName), Dynasty: strings.TrimSpace(r.Dynasty), Kind: file.Kind, Form: formFor(file.Kind, r.Content), Cipai: strings.TrimSpace(cipai), Lines: r.Content, Pinyin: r.Pinyin, Translation: strings.TrimSpace(r.Translation), Annotations: r.Annotation, Appreciation: strings.TrimSpace(r.Appreciation), Themes: themes, Collections: collections, AgeMin: ageMin, AgeMax: ageMax, PopularScore: score, ContentHash: contentHash(r.AuthorName, r.Title, r.Content), Source: model.SourceInfo{Name: "snowtraces/poetry-source", URL: "https://github.com/snowtraces/poetry-source", Commit: poetryCommit, SourceID: r.ID, LicenseNote: "项目代码为 MIT；公开渠道文本、现代译文和注释需按字段继续核查来源与授权。"}}
			if err := enc.Encode(item); err != nil {
				return stats, err
			}
			stats.Total++
			if hasPinyin(r.Pinyin) {
				stats.WithPinyin++
			}
			if item.Translation != "" {
				stats.WithTranslation++
			}
			if len(item.Annotations) > 0 {
				stats.WithAnnotations++
			}
			if item.Appreciation != "" {
				stats.WithAppreciation++
			}
			if hasPinyin(r.Pinyin) && item.Translation != "" && len(item.Annotations) > 0 {
				stats.CompleteLearningData++
			}
		}
	}
	if err := gz.Close(); err != nil {
		_ = temp.Close()
		return stats, err
	}
	if err := temp.Close(); err != nil {
		return stats, err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return stats, err
	}
	return stats, nil
}

func tuneFor(kind, title string) string {
	title = strings.TrimSpace(title)
	for _, separator := range []string{"（", "(", "·"} {
		if separator != "·" {
			if index := strings.Index(title, separator); index > 0 {
				title = strings.TrimSpace(title[:index])
			}
		}
	}
	parts := strings.Split(title, "·")
	if kind == "qu" && len(parts) >= 2 {
		return strings.TrimSpace(parts[0]) + "·" + strings.TrimSpace(parts[1])
	}
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return title
}

func hasPinyin(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}
func writeJSON(path string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fatal(err)
	}
	data = append(data, '\n')
	if err = os.WriteFile(path, data, 0o644); err != nil {
		fatal(err)
	}
}
func fileSHA(path string) string {
	f, err := os.Open(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
