package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
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

var famous = map[string]int{
	"李白|静夜思": 100, "王之涣|登鹳雀楼": 99, "李白|望庐山瀑布": 98, "李白|早发白帝城": 97,
	"李白|赠汪伦": 96, "李白|黄鹤楼送孟浩然之广陵": 95, "杜甫|望岳": 94, "杜甫|春夜喜雨": 93,
	"杜甫|闻官军收河南河北": 92, "杜甫|江南逢李龟年": 91, "杜甫|登高": 90, "张继|枫桥夜泊": 89,
	"孟郊|游子吟": 88,
}
var primarySchool = map[string]bool{"李白|静夜思": true, "王之涣|登鹳雀楼": true, "李白|望庐山瀑布": true, "李白|早发白帝城": true, "李白|赠汪伦": true, "李白|黄鹤楼送孟浩然之广陵": true, "杜甫|春夜喜雨": true, "杜甫|江南逢李龟年": true, "孟郊|游子吟": true, "张继|枫桥夜泊": true}

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
	patterns := []struct{ pattern, kind string }{{filepath.Join(*poetryRoot, "source", "诗", "唐", "poetry.唐.000?.json"), "poem"}, {filepath.Join(*poetryRoot, "source", "词", "宋", "ci.宋.000?.json"), "ci"}}
	items := []model.PoemPayload{}
	seen := map[string]bool{}
	for _, entry := range patterns {
		files, err := filepath.Glob(entry.pattern)
		if err != nil {
			fatal(err)
		}
		sort.Strings(files)
		for _, file := range files {
			records := loadSource(file)
			pinyinFile := strings.TrimSuffix(file, ".json") + ".pinyin.json"
			pinyin := loadPinyin(pinyinFile)
			for _, r := range records {
				if strings.TrimSpace(r.Translation) == "" || len(r.Annotation) == 0 {
					continue
				}
				r.Pinyin = pinyin[r.ID]
				if len(r.Content) == 0 || len(r.Pinyin) != len(r.Content) {
					continue
				}
				hash := contentHash(r.AuthorName, r.Title, r.Content)
				if seen[hash] {
					continue
				}
				seen[hash] = true
				key := contentKey(r.AuthorName, r.Content)
				tags := append([]string{}, selectedByContent[key]...)
				tags = append(tags, selectedByTitle[titleKey(r.AuthorName, r.Title)]...)
				cipai := ""
				if entry.kind == "ci" {
					cipai = cipaiByContent[key]
					if cipai == "" {
						cipai = r.Title
					}
				}
				collections, score := collectionsFor(r.AuthorName, r.Title, strings.Join(r.Content, ""), tags)
				themes := themesFor(r.Title, strings.Join(r.Content, ""), tags)
				ageMin, ageMax := ageFor(r.Content, r.Annotation)
				items = append(items, model.PoemPayload{ID: r.ID, Title: r.Title, Author: r.AuthorName, Dynasty: r.Dynasty, Kind: entry.kind, Form: formFor(entry.kind, r.Content), Cipai: cipai, Lines: r.Content, Pinyin: r.Pinyin, Translation: strings.TrimSpace(r.Translation), Annotations: r.Annotation, Appreciation: strings.TrimSpace(r.Appreciation), Themes: themes, Collections: collections, AgeMin: ageMin, AgeMax: ageMax, PopularScore: score, ContentHash: hash, Source: model.SourceInfo{Name: "snowtraces/poetry-source", URL: "https://github.com/snowtraces/poetry-source", Commit: *poetryCommit, SourceID: r.ID, LicenseNote: "项目代码为 MIT；公开渠道文本、现代译文和注释需按字段继续核查来源与授权。"}})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PopularScore == items[j].PopularScore {
			return items[i].ID < items[j].ID
		}
		return items[i].PopularScore > items[j].PopularScore
	})
	dataPath := filepath.Join(*outDir, "poems.jsonl.gz")
	if err := writeDataset(dataPath, items); err != nil {
		fatal(err)
	}
	digest := fileSHA(dataPath)
	manifest := model.DatasetManifest{Version: *version, GeneratedAt: time.Now().UTC(), Count: len(items), SHA256: digest, Sources: map[string]string{"snowtraces/poetry-source": *poetryCommit, "chinese-poetry/chinese-poetry": *chineseCommit}, Selection: "唐诗与宋词 0000–0009 分片中，同时具有非空白话译文、注释和行级拼音且结构校验通过的记录；精选集、词牌与知名度信号由 chinese-poetry 和人工名单补充。"}
	writeJSON(filepath.Join(*outDir, "manifest.json"), manifest)
	attribution := `# 数据来源与使用说明

- 主记录与拼音：snowtraces/poetry-source，固定提交 ` + *poetryCommit + `
- 精选集、词牌与知名度辅助信息：chinese-poetry/chinese-poetry，固定提交 ` + *chineseCommit + `

原诗词属于古代作品；仓库中的现代译文、注释、赏析等附加文本不因代码许可证而自动获得商业授权。本数据包保留字段级来源，当前用于个人学习型站点；对外商业发布前应继续完成文本来源审核，或替换为自有人工整理版本。
`
	if err := os.WriteFile(filepath.Join(*outDir, "ATTRIBUTION.md"), []byte(attribution), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("version=%s records=%d sha256=%s out=%s\n", *version, len(items), digest, *outDir)
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
func writeDataset(path string, items []model.PoemPayload) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	enc := json.NewEncoder(gz)
	enc.SetEscapeHTML(false)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return err
		}
	}
	return gz.Close()
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
	data, err := os.ReadFile(path)
	if err != nil {
		fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
