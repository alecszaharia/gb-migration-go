package global_blocks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type nodeIdSet struct {
	globalBlockNodeId        sql.NullInt64
	projectNodeId            sql.NullInt64
	metafieldMetaId          sql.NullInt64
	metafieldPositionId      sql.NullInt64
	metafieldMigratedRulesId sql.NullInt64
	metafieldApiVersionId    sql.NullInt64
	metafieldTagsId          sql.NullInt64
	metafieldCompileddataId  sql.NullInt64
	metafieldDependenciesId  sql.NullInt64
}

type repository struct {
	db                        *sql.DB
	nodeIdSet                 *nodeIdSet
	insertPageDataStm         *sql.Stmt
	insertCompiledDataStm     *sql.Stmt
	insertGlobalBlockStm      *sql.Stmt
	insertRuleStm             *sql.Stmt
	selectGlobalBlocksStm     *sql.Stmt
	fieldIdToCollectionTypeId map[int64]int64
	// pointer so copies made by withTx share the same lock as the shared map
	fieldIdToCollectionTypeMu *sync.RWMutex
}

type globalBlock struct {
	id                           sql.NullInt64
	projectId                    sql.NullInt64
	parentId                     sql.NullInt64
	authorId                     sql.NullInt64
	uid                          sql.NullString
	title                        sql.NullString
	status                       sql.NullString
	meta                         sql.NullString
	position                     sql.NullString
	rules                        sql.NullString
	tags                         sql.NullString
	compileddataMetafieldValueId sql.NullInt64
	dependencies                 sql.NullString
	createdAt                    sql.NullString
	updatedAt                    sql.NullString
}

func (s *repository) close() {
	s.insertPageDataStm.Close()
	s.insertCompiledDataStm.Close()
	s.insertGlobalBlockStm.Close()
	s.selectGlobalBlocksStm.Close()
}

// withTx returns a copy of the repository whose insert statements run inside tx.
func (s *repository) withTx(ctx context.Context, tx *sql.Tx) *repository {
	c := *s
	c.insertPageDataStm = tx.StmtContext(ctx, s.insertPageDataStm)
	c.insertCompiledDataStm = tx.StmtContext(ctx, s.insertCompiledDataStm)
	c.insertGlobalBlockStm = tx.StmtContext(ctx, s.insertGlobalBlockStm)
	c.insertRuleStm = tx.StmtContext(ctx, s.insertRuleStm)
	return &c
}

func (s *repository) insertPageData(ctx context.Context, projectId int64, dataId int64) (int64, error) {
	result, err := s.insertPageDataStm.ExecContext(ctx, projectId, dataId)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *repository) insertCompiledData(ctx context.Context, projectId int64, metaId int64) (int64, error) {
	result, err := s.insertCompiledDataStm.ExecContext(ctx, projectId, metaId)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *repository) insertGlobalBlock(ctx context.Context, projectId int64, page_data_id int64, compiled_data_id sql.NullInt64, dependencies sql.NullString, uid string, author_id int64, title string, status string, meta string, position string, tags string, created_at string, updated_at string) (int64, error) {
	result, err := s.insertGlobalBlockStm.ExecContext(ctx, projectId, page_data_id, compiled_data_id, dependencies, uid, author_id, title, status, meta, position, tags, created_at, updated_at)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

// nullableId maps the zero value of an unset foreign key to NULL; 0 is never a valid id and violates the FK constraints.
func nullableId(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// nullableString maps an unset string to NULL to match the nullable columns.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *repository) insertRule(ctx context.Context, r newRule) (int64, error) {
	result, err := s.insertRuleStm.ExecContext(ctx, r.global_block, r.project_id, nullableId(r.collection_item), nullableId(r.collection_type),
		nullableId(r.customer), nullableId(r.customer_group), r.mode, r.RuleType,
		nullableString(r.collection_type_slug), nullableString(r.external_type), nullableString(r.external_id),
		nullableId(r.collection_type_field), nullableId(r.field_value_item), time.Now(), time.Now())
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *repository) getGlobalBLock(ctx context.Context, id int64) (*globalBlock, error) {
	gb := globalBlock{}

	row := s.selectGlobalBlocksStm.QueryRowContext(ctx, id)
	if err := row.Scan(
		&gb.id,
		&gb.projectId,
		&gb.authorId,
		&gb.uid,
		&gb.title,
		&gb.status,
		&gb.meta,
		&gb.position,
		&gb.rules,
		&gb.tags,
		&gb.compileddataMetafieldValueId,
		&gb.dependencies,
		&gb.createdAt,
		&gb.updatedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to get row: %s", err)
	}

	return &gb, nil
}

func (s *repository) getGlobalBLocks(ctx context.Context, ids []int64) ([]globalBlock, error) {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")

	var query = fmt.Sprintf(`SELECT
				  d.id AS global_block_id,
				  d.parent_id AS project_id,
				  d.author_id,
				  d.uid,
				  IF(d.title <> '', d.title, 'Unnamed global block') AS title,
				  IF(d.status = 'draft', 'draft', 'published') AS status,
				  (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS meta,
				  (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS position,
				  IFNULL((SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d),'[]') AS rules,
				  (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS tags,
				  (SELECT id FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS compiledData_metafield_value_id,
				  (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS dependencies,
				  d.created_at,
				  d.updated_at
				FROM data d
				WHERE d.id IN (%s)`,
		s.nodeIdSet.metafieldMetaId.Int64,
		s.nodeIdSet.metafieldPositionId.Int64,
		s.nodeIdSet.metafieldMigratedRulesId.Int64,
		s.nodeIdSet.metafieldTagsId.Int64,
		s.nodeIdSet.metafieldCompileddataId.Int64,
		s.nodeIdSet.metafieldDependenciesId.Int64,
		placeholders)

	args := make([]any, 0)

	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("Faild to query the global blocks")
	}
	defer rows.Close()

	var gbs = make([]globalBlock, 0)

	for rows.Next() {
		var gb = globalBlock{}

		if err := rows.Scan(
			&gb.id,
			&gb.projectId,
			&gb.authorId,
			&gb.uid,
			&gb.title,
			&gb.status,
			&gb.meta,
			&gb.position,
			&gb.rules,
			&gb.tags,
			&gb.compileddataMetafieldValueId,
			&gb.dependencies,
			&gb.createdAt,
			&gb.updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to get row: %s", err)
		}

		gbs = append(gbs, gb)
	}

	return gbs, nil
}

func (s *repository) getTotalCount(ctx context.Context, startId int64) (int64, error) {

	stmtOut, err := s.db.PrepareContext(ctx, `
				SELECT COUNT(*)
                FROM metafield__int mi
                STRAIGHT_JOIN data d ON d.parent_id = mi.entity_id
                WHERE mi.metafield_id = ? AND mi.value = 2 and d.node_id=? and d.id > ?`)

	if err != nil {
		return 0, fmt.Errorf("failed to to prepare statement: %s", err)
	}

	defer stmtOut.Close()
	var total int64
	row := stmtOut.QueryRowContext(ctx, s.nodeIdSet.metafieldApiVersionId, s.nodeIdSet.globalBlockNodeId, startId)
	if err := row.Scan(&total); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get row: %s", err)
	}

	return total, nil
}

func (s *repository) getFailedTotalCount(ctx context.Context) (int64, error) {

	stmtOut, err := s.db.PrepareContext(ctx, `SELECT COUNT(*) FROM global_block_migration_failed`)

	if err != nil {
		return 0, fmt.Errorf("failed to to prepare statement: %s", err)
	}

	defer stmtOut.Close()
	var total int64
	row := stmtOut.QueryRowContext(ctx)
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to get row: %s", err)
	}

	return total, nil
}

func (s *repository) getGlobalBlocksIds(ctx context.Context, latestId int64, batch int) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.id
                FROM metafield__int mi
                STRAIGHT_JOIN data d ON d.parent_id = mi.entity_id
                WHERE mi.metafield_id = ? AND mi.value = 2 and d.node_id=? and d.id > ?
                ORDER BY d.id ASC
                LIMIT ?`, s.nodeIdSet.metafieldApiVersionId, s.nodeIdSet.globalBlockNodeId, latestId, batch)
	if err != nil {
		return nil, fmt.Errorf("Faild to query the global blocks")
	}

	var result = make([]int64, 0, batch)

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("failed to get row: %s", err)
			}
		}

		result = append(result, id)
	}
	defer rows.Close()
	return result, nil
}

func (s *repository) getFailedGlobalBlocksIds(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data_id FROM global_block_migration_failed`)
	if err != nil {
		return nil, fmt.Errorf("Faild to query the global blocks")
	}
	defer rows.Close()

	var result = make([]int64, 0, batch)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to get row: %s", err)
		}

		result = append(result, id)
	}

	return result, nil
}
func (s *repository) getCollectionTypeFromFieldTypeId(fieldTypeId int64) (int64, error) {
	s.fieldIdToCollectionTypeMu.RLock()
	cached, ok := s.fieldIdToCollectionTypeId[fieldTypeId]
	s.fieldIdToCollectionTypeMu.RUnlock()
	if ok {
		return cached, nil
	}

	stmtOut, err := s.db.Prepare(`SELECT collection_type_id FROM collection_type_field WHERE id = ?`)

	if err != nil {
		return 0, fmt.Errorf("failed to to prepare statement: %s", err)
	}

	defer stmtOut.Close()
	var collectionTypeId int64
	row := stmtOut.QueryRow(fieldTypeId)
	if err := row.Scan(&collectionTypeId); err != nil {
		return 0, fmt.Errorf("failed to get row: %s", err)
	}

	s.fieldIdToCollectionTypeMu.Lock()
	s.fieldIdToCollectionTypeId[fieldTypeId] = collectionTypeId
	s.fieldIdToCollectionTypeMu.Unlock()

	return collectionTypeId, nil
}

func getNodeIds(ctx context.Context, db *sql.DB, entity string) (*nodeIdSet, error) {

	idSet := nodeIdSet{}
	stmtOut, err := db.PrepareContext(ctx, "SELECT id, parent_id FROM node WHERE slug=?")
	if err != nil {
		return nil, fmt.Errorf("failed to to prepare statement: %s", err)
	}

	defer stmtOut.Close()

	row := stmtOut.QueryRowContext(ctx, entity)
	if err := row.Scan(&idSet.globalBlockNodeId, &idSet.projectNodeId); err != nil {

		return nil, fmt.Errorf("failed to get row: %s", err)
	}

	stmtOut2, err := db.PrepareContext(ctx, `SELECT 
			(SELECT id FROM metafield WHERE name = 'meta' and node_id = ?) as metafieldMetaId, 
			(SELECT id FROM metafield WHERE name = 'position' and node_id =?) as metafieldPositionId, 
			(SELECT id FROM metafield WHERE name = 'migrated_rules' and node_id = ?) as metafieldMigratedRulesId, 
			(SELECT id FROM metafield WHERE name = 'api_version' and node_id =?) as metafieldApiVersionId, 
			(SELECT id FROM metafield WHERE name = 'tags' and node_id = ?) as metafieldTagsId, 
			(SELECT id FROM metafield WHERE name = 'compiledData' and node_id = ?) as metafieldCompileddataId, 
			(SELECT id FROM metafield WHERE name = 'dependencies' and node_id =?) as metafieldDependenciesId
			`)
	if err != nil {
		return nil, fmt.Errorf("failed to to prepare statement: %s", err)
	}
	defer stmtOut2.Close()

	args := make([]any, 7)

	for i := range args {
		args[i] = idSet.globalBlockNodeId
	}

	args[3] = idSet.projectNodeId

	row2 := stmtOut2.QueryRowContext(ctx, args...)
	if err := row2.Scan(
		&idSet.metafieldMetaId,
		&idSet.metafieldPositionId,
		&idSet.metafieldMigratedRulesId,
		&idSet.metafieldApiVersionId,
		&idSet.metafieldTagsId,
		&idSet.metafieldCompileddataId,
		&idSet.metafieldDependenciesId,
	); err != nil {
		return nil, fmt.Errorf("failed to get row: %s", err)
	}

	return &idSet, nil
}

func newPrepareRepository(ctx context.Context, db *sql.DB) (*repository, error) {

	nodeIdSet, err := getNodeIds(ctx, db, "globalblock")

	if err != nil {
		fmt.Println("Failed to get the node and metafield ids")
		return nil, fmt.Errorf("Failed to get the node and metafield ids: %w", err)
	}

	insertPageData, err := db.PrepareContext(ctx, `INSERT INTO page_data (project_id,data) SELECT ?, d.body FROM data d WHERE d.id = ?`)
	if err != nil {
		return nil, err
	}
	insertCompiledData, err := db.PrepareContext(ctx, `INSERT INTO compiled_data (project_id,data,ttl) SELECT ?, mt.value, UNIX_TIMESTAMP()+7776000 FROM metafield__text mt WHERE mt.id = ?`)
	if err != nil {
		return nil, err
	}
	insertGlobalBlock, err := db.PrepareContext(ctx, `INSERT INTO global_block ( project_id,page_data_id, compiled_data_id,dependencies,uid,author_id,title,status,meta,position,tags,created_at,updated_at ) 
            												VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return nil, err
	}

	insertRuleStm, err := db.PrepareContext(ctx, `INSERT INTO rules 	
    		(global_block, project_id, collection_item, collection_type, customer, 
    		 customer_group, mode, type, collection_type_slug, external_type, 
    		 external_id, collection_type_field, field_value_item, 
    		 created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return nil, err
	}

	selectGlobalBlocks, err := db.PrepareContext(ctx,
		fmt.Sprintf(`SELECT
              d.id AS global_block_id,
              d.parent_id AS project_id,
              d.author_id,
              d.uid,
              IF(d.title <> '', d.title, 'Unnamed global block') AS title,
              IF(d.status = 'draft', 'draft', 'published') AS status,
              (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS meta,
              (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS position,
              IFNULL((SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d),'[]') AS rules,
              (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS tags,
              (SELECT id FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS compiledData_metafield_value_id,
              (SELECT value FROM metafield__text WHERE entity_id=d.id AND metafield_id=%d) AS dependencies,
              d.created_at,
              d.updated_at
            FROM data d
            WHERE d.id IN (?)`, nodeIdSet.metafieldMetaId.Int64, nodeIdSet.metafieldPositionId.Int64, nodeIdSet.metafieldMigratedRulesId.Int64, nodeIdSet.metafieldTagsId.Int64, nodeIdSet.metafieldCompileddataId.Int64, nodeIdSet.metafieldDependenciesId.Int64))
	if err != nil {
		return nil, err
	}

	return &repository{
		db:                        db,
		nodeIdSet:                 nodeIdSet,
		insertPageDataStm:         insertPageData,
		insertCompiledDataStm:     insertCompiledData,
		insertGlobalBlockStm:      insertGlobalBlock,
		insertRuleStm:             insertRuleStm,
		selectGlobalBlocksStm:     selectGlobalBlocks,
		fieldIdToCollectionTypeId: make(map[int64]int64),
		fieldIdToCollectionTypeMu: &sync.RWMutex{},
	}, nil
}
