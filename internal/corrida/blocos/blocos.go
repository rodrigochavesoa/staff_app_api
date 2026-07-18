package blocos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"staff_app/internal/daniels"
	"staff_app/internal/domain"
)

const (
	ModoTemplate       = "template"
	ModoBlocosCompleta = "blocos_completa"
	ModoSemanaASemana  = "semana_a_semana"
)

// Template is a clean-room Daniels-inspired workout template.
type Template struct {
	ID             string               `json:"id"`
	Nome           string               `json:"nome"`
	Categoria      string               `json:"categoria"`
	Nivel          []string             `json:"nivel"`
	ZonaPrincipal  string               `json:"zona_principal"`
	DuracaoMinutos float64              `json:"duracao_minutos"`
	Blocos         []domain.BlocoCorrida `json:"blocos"`
}

// LoadTemplates reads templates from a JSON file path.
// Only the known clean-room templates filename is accepted.
func LoadTemplates(path string) ([]Template, error) {
	cleaned := filepath.Clean(path)
	if filepath.Base(cleaned) != "templates_daniels_blocos.json" {
		return nil, fmt.Errorf("unsupported templates file")
	}
	// Path is server-controlled (handler default or test fixture), never request input.
	raw, err := os.ReadFile(cleaned) // #nosec G304 -- filename allowlisted; path not from client
	if err != nil {
		return nil, fmt.Errorf("reading templates: %w", err)
	}
	var templates []Template
	if err := json.Unmarshal(raw, &templates); err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("templates file is empty")
	}
	return templates, nil
}

// NormalizeModoGeracao maps request flags to a canonical modo_geracao value.
func NormalizeModoGeracao(usarBlocos bool, modo string) (string, error) {
	modo = strings.TrimSpace(strings.ToLower(modo))
	if !usarBlocos {
		if modo == ModoSemanaASemana || modo == "semana_a_semana" {
			return "", fmt.Errorf("modo_geracao semana_a_semana exige usar_blocos=true")
		}
		if modo == "" || modo == ModoTemplate || modo == "todas" {
			return ModoTemplate, nil
		}
		if modo == ModoBlocosCompleta {
			return "", fmt.Errorf("modo_geracao blocos_completa exige usar_blocos=true")
		}
		return "", fmt.Errorf("modo_geracao inválido: %s", modo)
	}

	switch modo {
	case "", "todas", ModoBlocosCompleta:
		return ModoBlocosCompleta, nil
	case ModoSemanaASemana:
		return ModoSemanaASemana, nil
	case ModoTemplate:
		return "", fmt.Errorf("usar_blocos=true é incompatível com modo_geracao=template")
	default:
		return "", fmt.Errorf("modo_geracao inválido: %s", modo)
	}
}

// FaseForWeek returns the block-mode phase label for a week number.
func FaseForWeek(semanaNumero, duracaoSemanas int) (fase string, categorias []string) {
	if duracaoSemanas < 1 {
		duracaoSemanas = 12
	}
	ratio := float64(semanaNumero) / float64(duracaoSemanas)
	switch {
	case ratio <= 0.3:
		return "Base", []string{"Easy", "Long"}
	case ratio <= 0.7:
		return "Construção", []string{"Threshold", "Interval", "Fartlek"}
	default:
		return "Específica", []string{"Repetition", "Interval", "Marathon", "Long"}
	}
}

func categoriasPorDistancia(distancia string) []string {
	switch distancia {
	case "5K":
		return []string{"Interval", "Repetition", "Fartlek", "Easy"}
	case "10K":
		return []string{"Threshold", "Interval", "Fartlek", "Easy"}
	case "21K":
		return []string{"Threshold", "Long", "Easy", "Fartlek"}
	case "42K":
		return []string{"Marathon", "Long", "Threshold", "Easy"}
	default:
		return []string{"Easy", "Threshold"}
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// SelectTemplates filters templates by level and race distance.
func SelectTemplates(all []Template, nivel, distanciaProva string) []Template {
	byNivel := make([]Template, 0, len(all))
	for _, t := range all {
		if containsStr(t.Nivel, nivel) {
			byNivel = append(byNivel, t)
		}
	}
	if len(byNivel) == 0 {
		byNivel = all
	}

	cats := categoriasPorDistancia(distanciaProva)
	selected := make([]Template, 0, len(byNivel))
	for _, t := range byNivel {
		if containsStr(cats, t.Categoria) {
			selected = append(selected, t)
		}
	}
	if len(selected) == 0 {
		for _, t := range byNivel {
			if t.Categoria == "Easy" {
				selected = append(selected, t)
			}
		}
	}
	if len(selected) == 0 {
		return byNivel
	}
	return selected
}

func formatPace(seconds int) string {
	if seconds <= 0 {
		return "05:30"
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func paceMapFromVDOT(vdot float64) map[string]string {
	zones := daniels.CalculateZones(vdot)
	out := map[string]string{
		"E":    formatPace(zones["E"].PaceAlvo),
		"M":    formatPace(zones["M"].PaceAlvo),
		"T":    formatPace(zones["T"].PaceAlvo),
		"I":    formatPace(zones["I"].PaceAlvo),
		"R":    formatPace(zones["R"].PaceAlvo),
		"Rest": "06:30",
	}
	return out
}

// ApplyPaces clones blocks and injects Daniels paces for atomic intensities.
func ApplyPaces(blocos []domain.BlocoCorrida, vdot float64) []domain.BlocoCorrida {
	paces := paceMapFromVDOT(vdot)
	return applyPacesRecursive(blocos, paces)
}

func applyPacesRecursive(in []domain.BlocoCorrida, paces map[string]string) []domain.BlocoCorrida {
	out := make([]domain.BlocoCorrida, len(in))
	for i, b := range in {
		cp := b
		if cp.Type == "atomic" {
			intensity := cp.Intensity
			if intensity == "" {
				intensity = "E"
			}
			if pace, ok := paces[intensity]; ok {
				cp.PaceMinKM = pace
			}
		}
		if len(cp.Content) > 0 {
			cp.Content = applyPacesRecursive(cp.Content, paces)
		}
		out[i] = cp
	}
	return out
}

// DurationMinutes sums duration across atomic and repeated blocks.
func DurationMinutes(blocos []domain.BlocoCorrida) float64 {
	var total float64
	for _, b := range blocos {
		switch b.Type {
		case "repeater":
			rep := b.Repeat
			if rep < 1 {
				rep = 1
			}
			total += float64(rep) * DurationMinutes(b.Content)
		default:
			total += b.DurationMin
		}
	}
	return total
}

// EstimateDistanceKM estimates distance from duration and primary zone pace.
func EstimateDistanceKM(durationMin float64, paceMMSS string) float64 {
	if durationMin <= 0 || paceMMSS == "" {
		return 0
	}
	var m, s int
	if _, err := fmt.Sscanf(paceMMSS, "%d:%d", &m, &s); err != nil || m < 0 || s < 0 || s >= 60 {
		return 0
	}
	paceSec := m*60 + s
	if paceSec <= 0 {
		return 0
	}
	km := (durationMin * 60) / float64(paceSec)
	return float64(int(km*10+0.5)) / 10
}

// IntensityDistribution counts atomic intensities including repeater expansion.
func IntensityDistribution(blocos []domain.BlocoCorrida) map[string]int {
	dist := map[string]int{}
	var walk func([]domain.BlocoCorrida, int)
	walk = func(items []domain.BlocoCorrida, mult int) {
		for _, b := range items {
			switch b.Type {
			case "repeater":
				rep := b.Repeat
				if rep < 1 {
					rep = 1
				}
				walk(b.Content, mult*rep)
			default:
				intensity := b.Intensity
				if intensity == "" {
					intensity = "E"
				}
				dist[intensity] += mult
			}
		}
	}
	walk(blocos, 1)
	return dist
}

// DowngradeHardIntensities replaces I/R with E for high clinical risk.
func DowngradeHardIntensities(blocos []domain.BlocoCorrida) []domain.BlocoCorrida {
	out := make([]domain.BlocoCorrida, len(blocos))
	for i, b := range blocos {
		cp := b
		if cp.Type == "atomic" && (cp.Intensity == "I" || cp.Intensity == "R") {
			cp.Intensity = "E"
			if cp.Description != "" {
				cp.Description = cp.Description + " (ajustado por risco clínico)"
			}
		}
		if len(cp.Content) > 0 {
			cp.Content = DowngradeHardIntensities(cp.Content)
		}
		out[i] = cp
	}
	return out
}

func cloneTemplate(t Template) Template {
	raw, _ := json.Marshal(t)
	var cp Template
	_ = json.Unmarshal(raw, &cp)
	return cp
}

// GenerateWeek builds one week of block-based workouts.
func GenerateWeek(templates []Template, semanaNumero, duracaoSemanas int, dias []int, vdot float64, nivel, distanciaProva string) (domain.SemanaJSON, error) {
	if len(dias) == 0 {
		return domain.SemanaJSON{}, fmt.Errorf("dias_semana vazio")
	}
	selected := SelectTemplates(templates, nivel, distanciaProva)
	if len(selected) == 0 {
		return domain.SemanaJSON{}, fmt.Errorf("nenhum template disponível")
	}

	fase, categoriasFase := FaseForWeek(semanaNumero, duracaoSemanas)
	faseTemplates := make([]Template, 0, len(selected))
	for _, t := range selected {
		if containsStr(categoriasFase, t.Categoria) {
			faseTemplates = append(faseTemplates, t)
		}
	}
	if len(faseTemplates) == 0 {
		faseTemplates = selected
	}

	treinos := make([]domain.TreinoJSON, 0, len(dias))
	var volume float64
	for i, dia := range dias {
		tpl := cloneTemplate(faseTemplates[i%len(faseTemplates)])
		blocosComPace := ApplyPaces(tpl.Blocos, vdot)
		dur := DurationMinutes(blocosComPace)
		if dur <= 0 {
			dur = tpl.DuracaoMinutos
		}
		pace := ""
		if len(blocosComPace) > 0 {
			for _, b := range blocosComPace {
				if b.Type == "atomic" && b.PaceMinKM != "" {
					pace = b.PaceMinKM
					break
				}
			}
		}
		if pace == "" {
			paces := paceMapFromVDOT(vdot)
			pace = paces[tpl.ZonaPrincipal]
			if pace == "" {
				pace = paces["E"]
			}
		}
		dist := EstimateDistanceKM(dur, pace)
		volume += dist

		treinos = append(treinos, domain.TreinoJSON{
			Dia:            dia,
			Tipo:           tpl.Nome,
			Distancia:      dist,
			Zona:           tpl.ZonaPrincipal,
			PaceAlvo:       pace,
			Descricao:      tpl.Nome,
			Nome:           tpl.Nome,
			TemplateID:     tpl.ID,
			ZonaPrincipal:  tpl.ZonaPrincipal,
			DuracaoMinutos: dur,
			Blocos:         blocosComPace,
		})
	}

	return domain.SemanaJSON{
		Numero:      semanaNumero,
		Fase:        fase,
		VolumeTotal: float64(int(volume*10+0.5)) / 10,
		Treinos:     treinos,
	}, nil
}

// GeneratePlano builds a full or first-week block plan.
func GeneratePlano(templates []Template, vdot float64, distanciaLabel string, distKM float64, duracaoSemanas int, nivel string, dias []int, modoGeracao string, zonas map[string]domain.ZoneDetails) (domain.PlanoDetalhado, error) {
	semanasParaGerar := duracaoSemanas
	if modoGeracao == ModoSemanaASemana {
		semanasParaGerar = 1
	}

	semanas := make([]domain.SemanaJSON, 0, semanasParaGerar)
	for n := 1; n <= semanasParaGerar; n++ {
		semana, err := GenerateWeek(templates, n, duracaoSemanas, dias, vdot, nivel, distanciaLabel)
		if err != nil {
			return domain.PlanoDetalhado{}, err
		}
		semanas = append(semanas, semana)
	}

	return domain.PlanoDetalhado{
		VDOT:                   vdot,
		DistanciaProva:         distKM,
		DuracaoSemanas:         duracaoSemanas,
		DiasSemanaSelecionados: dias,
		Zonas:                  zonas,
		Semanas:                semanas,
		Tipo:                   "blocos_dinamicos",
		ModoGeracao:            modoGeracao,
		SemanasGeradas:         len(semanas),
	}, nil
}

// PreviewDay holds a single-day preview used by gerar-blocos.
type PreviewDay struct {
	Dia            int                  `json:"dia"`
	Nome           string               `json:"nome"`
	ZonaPrincipal  string               `json:"zona_principal"`
	DuracaoMinutos float64              `json:"duracao_minutos"`
	Blocos         []domain.BlocoCorrida `json:"blocos"`
}

// GeneratePreview builds a one-week preview without persisting a plan.
func GeneratePreview(templates []Template, vdot float64, distancia, nivel string, diasSemana int) ([]PreviewDay, float64, map[string]int, error) {
	if diasSemana < 2 || diasSemana > 7 {
		return nil, 0, nil, fmt.Errorf("dias_semana deve estar entre 2 e 7")
	}
	dias := make([]int, diasSemana)
	for i := range dias {
		dias[i] = i + 1
	}
	semana, err := GenerateWeek(templates, 1, 12, dias, vdot, nivel, distancia)
	if err != nil {
		return nil, 0, nil, err
	}
	out := make([]PreviewDay, 0, len(semana.Treinos))
	var total float64
	distAll := map[string]int{}
	for _, t := range semana.Treinos {
		out = append(out, PreviewDay{
			Dia:            t.Dia,
			Nome:           t.Nome,
			ZonaPrincipal:  t.ZonaPrincipal,
			DuracaoMinutos: t.DuracaoMinutos,
			Blocos:         t.Blocos,
		})
		total += t.DuracaoMinutos
		for k, v := range IntensityDistribution(t.Blocos) {
			distAll[k] += v
		}
	}
	return out, total, distAll, nil
}
