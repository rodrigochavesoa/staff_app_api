package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
)

func TestAlunoRepositoryCRUD(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-aluno-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	var ctx context.Context = t.Context()

	repo := NewAlunoRepository(db)

	aprovadoEm := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	aluno1 := &domain.Aluno{
		Nome:                 "Bruno Bruno",
		Idade:                28,
		Sexo:                 "M",
		Email:                "bruno@test.com",
		Telefone:             "11999999999",
		Objetivo:             "Correr maratona",
		ExclusoesPermanentes: "Nenhuma",
		Turma:                "A",
		PlanoPago:            true,
		PlanoAtivo:           true,
		CadastroAprovado:     true,
		CadastroAprovadoEm:   &aprovadoEm,
		Ativo:                true,
	}

	// 1. Create Aluno
	err = repo.Create(ctx, aluno1)
	if err != nil {
		t.Fatalf("failed to create aluno: %v", err)
	}
	if aluno1.ID == 0 {
		t.Error("expected populated ID for aluno1")
	}

	// 2. GetByID
	got, err := repo.GetByID(ctx, aluno1.ID)
	if err != nil {
		t.Fatalf("failed to get aluno: %v", err)
	}
	if got.Nome != "Bruno Bruno" || got.Idade != 28 || got.Email != "bruno@test.com" || !got.Ativo {
		t.Errorf("unexpected aluno details retrieved: %+v", got)
	}
	if got.CadastroAprovadoEm == nil || got.CadastroAprovadoEm.Format(time.RFC3339) != aprovadoEm.Format(time.RFC3339) {
		t.Errorf("expected CadastroAprovadoEm to be %+v, got %+v", aprovadoEm, got.CadastroAprovadoEm)
	}

	// 3. Update Aluno
	got.Nome = "Bruno Editado"
	got.Idade = 29
	err = repo.Update(ctx, got)
	if err != nil {
		t.Fatalf("failed to update aluno: %v", err)
	}

	gotEdited, err := repo.GetByID(ctx, aluno1.ID)
	if err != nil {
		t.Fatalf("failed to get updated aluno: %v", err)
	}
	if gotEdited.Nome != "Bruno Editado" || gotEdited.Idade != 29 {
		t.Errorf("failed to verify updated data: %+v", gotEdited)
	}

	// 4. Create another aluno for list checking
	aluno2 := &domain.Aluno{
		Nome:     "Ana Ana",
		Idade:    24,
		Sexo:     "F",
		Email:    "ana@test.com",
		Objetivo: "Perder peso",
		Ativo:    true,
	}
	err = repo.Create(ctx, aluno2)
	if err != nil {
		t.Fatalf("failed to create aluno2: %v", err)
	}

	// 5. List (default active only)
	activeList, err := repo.List(ctx, "", false)
	if err != nil {
		t.Fatalf("failed to list active students: %v", err)
	}
	if len(activeList) != 2 {
		t.Errorf("expected 2 active students, got %d", len(activeList))
	}
	// Verify sorting by name ASC (Ana first, Bruno second)
	if activeList[0].Nome != "Ana Ana" || activeList[1].Nome != "Bruno Editado" {
		t.Errorf("incorrect list ordering or names: %+v", activeList)
	}

	// 6. Soft Delete Aluno
	err = repo.Delete(ctx, aluno2.ID)
	if err != nil {
		t.Fatalf("failed to soft delete aluno2: %v", err)
	}

	// Verify it's no longer in active list
	activeListAfter, err := repo.List(ctx, "", false)
	if err != nil {
		t.Fatalf("failed to list active students: %v", err)
	}
	if len(activeListAfter) != 1 {
		t.Errorf("expected 1 active student after delete, got %d", len(activeListAfter))
	}
	if activeListAfter[0].ID != aluno1.ID {
		t.Errorf("expected remaining student to be Bruno, got %+v", activeListAfter[0])
	}

	// Verify it IS in list when includeInactives is true
	allList, err := repo.List(ctx, "", true)
	if err != nil {
		t.Fatalf("failed to list all students: %v", err)
	}
	if len(allList) != 2 {
		t.Errorf("expected 2 students total, got %d", len(allList))
	}
	// Verify sorting when inactives included (ativo DESC, nome ASC -> Bruno first because active, Ana second because inactive)
	if allList[0].Nome != "Bruno Editado" || allList[1].Nome != "Ana Ana" {
		t.Errorf("incorrect inactives list ordering: %+v", allList)
	}

	// 7. Search filter checking
	searchList1, err := repo.List(ctx, "Editado", false)
	if err != nil {
		t.Fatalf("failed searching active: %v", err)
	}
	if len(searchList1) != 1 || searchList1[0].Nome != "Bruno Editado" {
		t.Errorf("search failed to find Bruno: %+v", searchList1)
	}

	searchList2, err := repo.List(ctx, "Ana", true)
	if err != nil {
		t.Fatalf("failed searching with inactives: %v", err)
	}
	if len(searchList2) != 1 || searchList2[0].Nome != "Ana Ana" {
		t.Errorf("search failed to find Ana: %+v", searchList2)
	}

	// 8. Reactivate Aluno
	err = repo.Reactivate(ctx, aluno2.ID)
	if err != nil {
		t.Fatalf("failed to reactivate student: %v", err)
	}

	reactivatedList, err := repo.List(ctx, "", false)
	if err != nil {
		t.Fatalf("failed to list active students after reactivation: %v", err)
	}
	if len(reactivatedList) != 2 {
		t.Errorf("expected 2 active students again, got %d", len(reactivatedList))
	}

	// Reactivate active student should fail with sql.ErrNoRows
	err = repo.Reactivate(ctx, aluno1.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows when reactivating already active student, got %v", err)
	}
}

func TestAlunoRepositoryNullableFields(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-aluno-null-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	var ctx context.Context = t.Context()

	// 1. Manually insert Aluno with NULL in all nullable fields
	query := `
		INSERT INTO alunos (
			id, nome, idade, sexo, email, telefone, objetivo, exclusoes_permanentes, turma, usuario_id,
			plano_id, plano_valor, plano_pago, plano_ativo, plano_inicio, plano_fim,
			cadastro_aprovado, cadastro_aprovado_por, cadastro_aprovado_em, pre_registro_id, ativo
		) VALUES (10, 'Null Aluno', 40, 'F', 'null@test.com', NULL, NULL, NULL, NULL, NULL,
		          NULL, NULL, 0, 0, NULL, NULL, 0, NULL, NULL, NULL, 1)
	`
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		t.Fatalf("failed to insert null student manually: %v", err)
	}

	repo := NewAlunoRepository(db)

	// 2. Fetch using GetByID
	got, err := repo.GetByID(ctx, 10)
	if err != nil {
		t.Fatalf("failed to GetByID student with NULL fields: %v", err)
	}

	if got.Nome != "Null Aluno" || got.Email != "null@test.com" {
		t.Errorf("unexpected retrieved non-null details: %+v", got)
	}

	// Verify scanned strings defaults to empty string
	if got.Telefone != "" || got.Objetivo != "" || got.ExclusoesPermanentes != "" || got.Turma != "" {
		t.Errorf("expected nullable string fields to scan as empty strings, got: Telefone=%q, Objetivo=%q, Exclusoes=%q, Turma=%q",
			got.Telefone, got.Objetivo, got.ExclusoesPermanentes, got.Turma)
	}

	// Verify scanned pointers are nil
	if got.UsuarioID != nil || got.PlanoID != nil || got.PlanoValor != nil || got.PlanoInicio != nil || got.PlanoFim != nil || got.CadastroAprovadoEm != nil {
		t.Errorf("expected nullable pointer fields to scan as nil, got: UsuarioID=%v, PlanoID=%v, PlanoValor=%v, PlanoInicio=%v, PlanoFim=%v, AprovadoEm=%v",
			got.UsuarioID, got.PlanoID, got.PlanoValor, got.PlanoInicio, got.PlanoFim, got.CadastroAprovadoEm)
	}

	// 3. Fetch using List
	list, err := repo.List(ctx, "Null", false)
	if err != nil {
		t.Fatalf("failed to List student with NULL fields: %v", err)
	}

	if len(list) != 1 || list[0].ID != 10 {
		t.Errorf("expected to find Null Aluno in search list, got: %+v", list)
	}
}

func TestAlunoRepositoryGetByUsuarioID(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-aluno-usuario-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	ctx := t.Context()
	repo := NewAlunoRepository(db)

	got, err := repo.GetByUsuarioID(ctx, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil aluno, got %+v", got)
	}

	userRepo := NewUserRepository(db)
	user := &domain.User{
		Username:     "linked-user",
		Email:        "linked-user@example.com",
		PasswordHash: "hash",
		NomeCompleto: "Linked User",
		IsAdmin:      false,
		Ativo:        true,
		Aprovado:     true,
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	userID := user.ID
	aluno := &domain.Aluno{
		Nome:      "Linked Student",
		Idade:     30,
		Sexo:      "M",
		Email:     "linked@example.com",
		UsuarioID: &userID,
		Ativo:     true,
	}
	if err := repo.Create(ctx, aluno); err != nil {
		t.Fatalf("failed to create aluno: %v", err)
	}

	got, err = repo.GetByUsuarioID(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Nome != "Linked Student" || got.UsuarioID == nil || *got.UsuarioID != userID {
		t.Fatalf("unexpected aluno: %+v", got)
	}
}
