// Package importer 将 Archive 导出的 dump（zip 或解压目录）流式导入 SQLite。
//
// 特点：
//   - 直接从 zip 内流式读取 jsonlines，无需解压到磁盘
//   - 全量重建（DROP + 重建），每周导出均为全量快照，无增量合并问题
//   - 批量事务提交 + WAL，兼顾导入速度与原子性
//   - 同步写入 FTS 虚拟表，导入完成即可搜索
package importer

import (
	"archive/zip"
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bangumi-subject-go/internal/db"
	"bangumi-subject-go/internal/model"
)

// Stats 导入统计。
type Stats struct {
	Subjects          int64
	Persons           int64
	Characters        int64
	Episodes          int64
	SubjectRelations  int64
	SubjectPersons    int64
	SubjectCharacters int64
	PersonCharacters  int64
	PersonRelations   int64
}

// Total 所有表行数之和。
func (s Stats) Total() int64 {
	return s.Subjects + s.Persons + s.Characters + s.Episodes +
		s.SubjectRelations + s.SubjectPersons + s.SubjectCharacters +
		s.PersonCharacters + s.PersonRelations
}

const (
	// 每个文件内的事务批次大小
	batchSize = 50000
	// 日志打印间隔
	logEvery = 100000
)

// Import 全量导入：删除旧表 -> 建裸表 -> 按依赖顺序导入各 jsonlines 文件
// -> 最后统一创建索引并填充 FTS（大批量装载的最快路径）。
// src 可以是 .zip 文件或包含 jsonlines 文件的目录。
// limit > 0 时每个文件最多导入 limit 行（用于测试）。
func Import(ctx context.Context, conn *sql.DB, src string, limit int64) (*Stats, error) {
	if err := db.DropAllTables(conn); err != nil {
		return nil, fmt.Errorf("清理旧数据: %w", err)
	}
	if err := db.InitSchema(conn); err != nil {
		return nil, fmt.Errorf("建表: %w", err)
	}

	stats := &Stats{}
	start := time.Now()

	// 按依赖顺序导入（先主体表，再关联表）
	steps := []struct {
		file  string
		table string
		load  func(context.Context, *json.Decoder, *sql.DB, int64) (int64, error)
	}{
		{"subject.jsonlines", "subjects", importSubjects},
		{"person.jsonlines", "persons", importPersons},
		{"character.jsonlines", "characters", importCharacters},
		{"episode.jsonlines", "episodes", importEpisodes},
		{"subject-relations.jsonlines", "subject_relations", importSubjectRelations},
		{"subject-persons.jsonlines", "subject_persons", importSubjectPersons},
		{"subject-characters.jsonlines", "subject_characters", importSubjectCharacters},
		{"person-characters.jsonlines", "person_characters", importPersonCharacters},
		{"person-relations.jsonlines", "person_relations", importPersonRelations},
	}

	counts := map[string]*int64{
		"subjects":           &stats.Subjects,
		"persons":            &stats.Persons,
		"characters":         &stats.Characters,
		"episodes":           &stats.Episodes,
		"subject_relations":  &stats.SubjectRelations,
		"subject_persons":    &stats.SubjectPersons,
		"subject_characters": &stats.SubjectCharacters,
		"person_characters":  &stats.PersonCharacters,
		"person_relations":   &stats.PersonRelations,
	}

	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		n, err := importFile(ctx, conn, src, step.file, limit, step.load)
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("跳过 %-25s（文件不存在）", step.file)
			continue
		}
		if err != nil {
			return stats, fmt.Errorf("导入 %s: %w", step.file, err)
		}
		*counts[step.table] = n
		log.Printf("导入完成 %-25s %10d 行 (%v)", step.file, n, time.Since(start).Round(time.Second))
	}

	// 数据装载完毕：统一建索引 + 集合式填充 FTS（导入期间不维护任何索引）
	log.Printf("开始创建索引与 FTS 全文索引...")
	idxStart := time.Now()
	if err := db.FinalizeSchema(conn); err != nil {
		return stats, fmt.Errorf("创建索引/FTS: %w", err)
	}
	log.Printf("索引与 FTS 构建完成 (%v)", time.Since(idxStart).Round(time.Second))

	log.Printf("全部导入完成：共 %d 行，耗时 %v（含索引 %v）",
		stats.Total(), time.Since(start).Round(time.Second), time.Since(idxStart).Round(time.Second))
	return stats, nil
}

// importFile 打开 src 中名为 filename 的 jsonlines 数据源并流式导入。
// src 为 zip 时读取 zip 内条目，为目录时读取目录下文件。
func importFile(ctx context.Context, conn *sql.DB, src, filename string, limit int64, load func(context.Context, *json.Decoder, *sql.DB, int64) (int64, error)) (int64, error) {
	r, err := openJSONLines(src, filename)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	dec := json.NewDecoder(bufio.NewReaderSize(r, 1<<20))
	return load(ctx, dec, conn, limit)
}

// openJSONLines 打开 jsonlines 数据源（zip 内条目或目录下文件）。
// 目录模式下文件缺失时返回 os.ErrNotExist。
func openJSONLines(src, filename string) (io.ReadCloser, error) {
	if strings.HasSuffix(strings.ToLower(src), ".zip") {
		zr, err := zip.OpenReader(src)
		if err != nil {
			return nil, fmt.Errorf("打开 zip: %w", err)
		}
		var f *zip.File
		for _, zf := range zr.File {
			if zf.Name == filename {
				f = zf
				break
			}
		}
		if f == nil {
			zr.Close()
			return nil, fmt.Errorf("zip 中找不到 %s", filename)
		}
		r, err := f.Open()
		if err != nil {
			zr.Close()
			return nil, fmt.Errorf("打开 zip 内 %s: %w", filename, err)
		}
		return &zipReadCloser{r: r, zr: zr}, nil
	}
	f, err := os.Open(filepath.Join(src, filename))
	if err != nil {
		return nil, err
	}
	return f, nil
}

// zipReadCloser 同时关闭 zip 内条目与 zip 文件本身。
type zipReadCloser struct {
	r  io.ReadCloser
	zr *zip.ReadCloser
}

func (z *zipReadCloser) Read(p []byte) (int, error) { return z.r.Read(p) }

func (z *zipReadCloser) Close() error {
	z.r.Close()
	return z.zr.Close()
}

// importSubjects 导入 subjects 表（FTS 由导入完成后的 FinalizeSchema 集合式填充）。
func importSubjects(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO subjects
		(id, type, name, name_cn, infobox, platform, summary, nsfw, date, favorite, series, tags, score, score_details, rank, meta_tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.Subject{} }, func(v any) error {
		s := v.(*model.Subject)
		fav, _ := json.Marshal(s.Favorite)
		tags, _ := json.Marshal(s.Tags)
		sd, _ := json.Marshal(s.ScoreDetails)
		mt, _ := json.Marshal(s.MetaTags)
		_, err := insert.Exec(s.ID, s.Type, s.Name, s.NameCN, s.Infobox, s.Platform, s.Summary,
			bool2int(s.NSFW), s.Date, string(fav), bool2int(s.Series), string(tags),
			s.Score, string(sd), s.Rank, string(mt))
		return err
	}, insert)
}

func importPersons(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO persons (id, name, type, career, infobox, summary, comments, collects)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.Person{} }, func(v any) error {
		p := v.(*model.Person)
		career, _ := json.Marshal(p.Career)
		_, err := insert.Exec(p.ID, p.Name, p.Type, string(career), p.Infobox, p.Summary, p.Comments, p.Collects)
		return err
	}, insert)
}

func importCharacters(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO characters (id, role, name, infobox, summary, comments, collects)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.Character{} }, func(v any) error {
		c := v.(*model.Character)
		_, err := insert.Exec(c.ID, c.Role, c.Name, c.Infobox, c.Summary, c.Comments, c.Collects)
		return err
	}, insert)
}

func importEpisodes(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO episodes (id, name, name_cn, description, airdate, disc, duration, subject_id, sort, type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.Episode{} }, func(v any) error {
		e := v.(*model.Episode)
		_, err := insert.Exec(e.ID, e.Name, e.NameCN, e.Description, e.Airdate, e.Disc, e.Duration, e.SubjectID, e.Sort, e.Type)
		return err
	}, insert)
}

func importSubjectRelations(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO subject_relations (subject_id, relation_type, related_subject_id, "order")
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.SubjectRelation{} }, func(v any) error {
		r := v.(*model.SubjectRelation)
		_, err := insert.Exec(r.SubjectID, r.RelationType, r.RelatedSubjectID, r.Order)
		return err
	}, insert)
}

func importSubjectPersons(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO subject_persons (subject_id, person_id, position, appear_eps)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.SubjectPerson{} }, func(v any) error {
		p := v.(*model.SubjectPerson)
		_, err := insert.Exec(p.SubjectID, p.PersonID, p.Position, p.AppearEps)
		return err
	}, insert)
}

func importSubjectCharacters(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO subject_characters (character_id, subject_id, type, "order")
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.SubjectCharacter{} }, func(v any) error {
		c := v.(*model.SubjectCharacter)
		_, err := insert.Exec(c.CharacterID, c.SubjectID, c.Type, c.Order)
		return err
	}, insert)
}

func importPersonCharacters(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO person_characters (person_id, subject_id, character_id, type, summary)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.PersonCharacter{} }, func(v any) error {
		p := v.(*model.PersonCharacter)
		_, err := insert.Exec(p.PersonID, p.SubjectID, p.CharacterID, p.Type, p.Summary)
		return err
	}, insert)
}

func importPersonRelations(ctx context.Context, dec *json.Decoder, conn *sql.DB, limit int64) (int64, error) {
	insert, err := conn.Prepare(`INSERT INTO person_relations (person_type, person_id, related_person_id, relation_type, spoiler, ended)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	return streamDecode(ctx, conn, dec, limit, func() any { return &model.PersonRelation{} }, func(v any) error {
		r := v.(*model.PersonRelation)
		_, err := insert.Exec(r.PersonType, r.PersonID, r.RelatedPersonID, r.RelationType, bool2int(r.Spoiler), bool2int(r.Ended))
		return err
	}, insert)
}

// streamDecode 流式解码 jsonlines，按批次提交事务。
// newValue 返回对应模型的新指针（如 &model.Subject{}），handle 负责落库。
// 事务中的 prepared statement 使用 tx.Stmt 包装，避免连接漂移。
func streamDecode(ctx context.Context, conn *sql.DB, dec *json.Decoder, limit int64,
	newValue func() any, handle func(any) error, stmts ...*sql.Stmt) (int64, error) {
	var (
		n      int64
		batch  int
		tx     *sql.Tx
		stmtTx []*sql.Stmt
	)

	begin := func() error {
		var err error
		tx, err = conn.Begin()
		if err != nil {
			return err
		}
		stmtTx = make([]*sql.Stmt, 0, len(stmts))
		for _, s := range stmts {
			ts := tx.StmtContext(ctx, s)
			stmtTx = append(stmtTx, ts)
		}
		return nil
	}
	commit := func() error {
		if tx == nil {
			return nil
		}
		for _, s := range stmtTx {
			s.Close()
		}
		err := tx.Commit()
		tx = nil
		return err
	}

	if err := begin(); err != nil {
		return 0, err
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = commit()
			return n, err
		}
		if limit > 0 && n >= limit {
			break
		}
		v := newValue()
		if err := dec.Decode(v); err != nil {
			if err == io.EOF {
				break
			}
			return n, err
		}
		if err := handle(v); err != nil {
			return n, err
		}
		n++
		batch++
		if batch >= batchSize {
			if err := commit(); err != nil {
				return n, err
			}
			if err := begin(); err != nil {
				return n, err
			}
			batch = 0
		}
	}
	if err := commit(); err != nil {
		return n, err
	}
	return n, nil
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}
