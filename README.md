# kids-poetry-be

“童诗小书房”后端，基于 Go 1.25、go-zero REST 和 MySQL。它只保存应用代码、数据结构和整理工具；诗词源文件与生成后的数据包不会进入本仓库。

## 能力

- 按关键词、朝代、作者、题目、内容类型、体裁、题材、词牌和精选集检索
- “流传最广”“小学精选”“经典选本”等独立推荐集合
- 返回正文、行级拼音、注释、白话译文、赏析和字段级来源
- gzip JSONL 百条批量幂等导入，记录数据集版本、对象路径、条数和 SHA-256
- MySQL ngram 中文全文索引：两字以上按短语检索，单字走题目前缀/诗人精确索引；列表先排 ID 再回表，避免读取无关大字段
- 题材与精选集使用标签索引计数，聚合维度进程内预热缓存；单字题目/作者前缀使用覆盖索引合并；`featured?random=true` 可从精选名篇中随机取一首
- `/api/v1/healthz` 数据库健康检查

## 数据边界

整理工具从固定 Git 提交读取 `snowtraces/poetry-source` 的诗、词、曲全部分片，并用 `chinese-poetry/chinese-poetry` 补充精选集、词牌和知名度信号。当前全量快照扫描 531,002 条源记录，跳过 1 条空正文，生成 531,001 条可阅读记录；所有作品有正文拼音，译文、注释和赏析按源数据实际覆盖保留，接口不会把缺失字段伪装成完整译注。

多数古代诗词原文已进入公共领域，但仓库也可能含近现代作品；现代译文、注释和赏析更不因代码仓库许可证而自动获得商业授权。当前数据集适合个人学习型站点；商业化前必须按作品和附加文本完成来源审核，或替换为自有整理版本。

## 本地运行

```bash
go test ./...
mysql < deploy/sql/schema.sql
POETRY_DB_DSN='user:password@tcp(127.0.0.1:3306)/kids_poetry?charset=utf8mb4&parseTime=true' \
  go run ./cmd/importer -source file -file /path/to/poems.jsonl.gz -version 2026-08-30.v1 \
  -count '<manifest count>' -sha256 '<manifest sha256>' -prune
POETRY_DB_DSN='user:password@tcp(127.0.0.1:3306)/kids_poetry?charset=utf8mb4&parseTime=true' \
POETRY_DATASET_VERSION='2026-08-30.v1' \
  go run ./cmd/server -f etc/backend.example.yaml
```

## 数据整理

源数据放在仓库外，输出目录也必须放在仓库外：

```bash
go run ./cmd/prepare-data \
  -poetry-source /external/poetry-source \
  -chinese-poetry /external/chinese-poetry \
  -out /external/release \
  -version 2026-08-30.v1
```

输出包含 `poems.jsonl.gz`、`manifest.json` 和 `ATTRIBUTION.md`。`manifest.json` 同时记录源条数、跳过条数以及拼音、译文、注释、赏析覆盖数。推荐通过 MinIO S3 API 上传到私有 bucket，再由 importer 从对象存储导入；不要直接写 MinIO 的磁盘目录。完整快照导入时传入 manifest 的条数和 SHA-256，并显式开启 `-prune`，校验与旧记录清理会在同一事务提交前完成。首次全量导入完成后，importer 会自动创建 MySQL `ngram` 全文索引，避免建索引拖慢 53 万条初始写入。

## API

| 路径 | 说明 |
| --- | --- |
| `GET /api/v1/healthz` | 健康检查 |
| `GET /api/v1/meta` | 数据集版本与条数 |
| `GET /api/v1/facets` | 朝代、作者、体裁、题材、词牌等聚合 |
| `GET /api/v1/poems` | 多条件检索与分页 |
| `GET /api/v1/poems/:id` | 诗词全文、拼音、译注与来源 |
| `GET /api/v1/featured` | 按精选集推荐 |

部署时由同源 Nginx 将 `/poetry/api/` 转发到后端，前端不会接触数据库或 MinIO 凭据。

常规构建使用多阶段 `Dockerfile`。受限网络环境也可以先交叉编译到已忽略的 `dist/linux-amd64/`，再用 `Dockerfile.release` 生成显式面向 linux/amd64、以非 root 用户运行的轻量 Alpine 镜像；发布镜像包含 API、importer 和一次性 MinIO 同步工具三个静态二进制。同步工具只通过 S3 API 建 bucket 和上传版本化对象，不直接写 MinIO 数据目录。
