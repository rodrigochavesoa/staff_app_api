package domain

import "time"

type Anamnese struct {
	ID                        int64      `json:"id"`
	AlunoID                   *int64     `json:"aluno_id"`
	DataNascimento            string     `json:"data_nascimento"`
	Idade                     int        `json:"idade"`
	Sexo                      string     `json:"sexo"`
	Altura                    float64    `json:"altura"`
	Peso                      float64    `json:"peso"`
	Telefone                  string     `json:"telefone"`
	Email                     string     `json:"email"`
	Patologias                string     `json:"patologias"`
	Medicamentos              string     `json:"medicamentos"`
	LesoesAtuais              string     `json:"lesoes_atuais"`
	DoresCronicas             string     `json:"dores_cronicas"`
	ParqDoencaCardiaca        int        `json:"parq_doenca_cardiaca"`
	ParqDorPeito              int        `json:"parq_dor_peito"`
	ParqTontura               int        `json:"parq_tontura"`
	ParqProblemaOsseo         int        `json:"parq_problema_osseo"`
	ParqMedicamentoPressao    int        `json:"parq_medicamento_pressao"`
	ParqImpedimentoActivity   int        `json:"parq_impedimento_activity"`
	ExperienciaTreino         string     `json:"experiencia_treino"`
	ObjetivoPrincipal         string     `json:"objetivo_principal"`
	ContatoEmergenciaNome     string     `json:"contato_emergencia_nome"`
	ContatoEmergenciaTelefone string     `json:"contato_emergencia_telefone"`
	RiskScoreCached           float64    `json:"risk_score_cached"`
	PreenchidoPor             string     `json:"preenchido_por"`
	Ativa                     bool       `json:"ativa"`
	CriadoEm                  time.Time  `json:"criado_em"`
	PreRegistroID             *int64     `json:"pre_registro_id"`
	StatusAprovacao           string     `json:"status_aprovacao"` // 'pendente', 'aprovada', 'rejeitada'
	AprovadoPor               *string    `json:"aprovado_por"`
	AprovadoEm                *time.Time `json:"aprovado_em"`
	MotivoRejeicao            *string    `json:"motivo_rejeicao"`
	TokenOrigem               *string    `json:"token_origem"`
}

type AnamneseToken struct {
	ID            int64      `json:"id"`
	Token         string     `json:"token"`
	PreRegistroID *int64     `json:"pre_registro_id"`
	ExpiraEm      time.Time  `json:"expira_em"`
	Usado         bool       `json:"usado"`
	AlunoID       *int64     `json:"aluno_id"`
	AlunoNome     string     `json:"aluno_nome"`
	AlunoEmail    string     `json:"aluno_email"`
	CriadoEm      time.Time  `json:"criado_em"`
	CriadoPor     string     `json:"criado_por"`
	IpOrigem      string     `json:"ip_origem"`
	UsadoEm       *time.Time `json:"usado_em"`
	IpSubmissao   string     `json:"ip_submissao"`
	AnamneseID    *int64     `json:"anamnese_id"`
}

type AnamneseTokenAudit struct {
	ID            int64     `json:"id"`
	Token         string    `json:"token"`
	AlunoID       *int64    `json:"aluno_id"`
	PreRegistroID *int64    `json:"pre_registro_id"`
	Evento        string    `json:"evento"` // 'GERADO', 'ENVIADO_EMAIL', 'VISUALIZADO', 'SUBMETIDO', 'EXPIRADO'
	Ip            string    `json:"ip"`
	UserAgent     string    `json:"user_agent"`
	Detalhes      string    `json:"detalhes"`
	DataEvento    time.Time `json:"data_evento"`
}
