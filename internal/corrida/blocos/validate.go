package blocos

import (
	"fmt"
	"strings"

	"staff_app/internal/domain"
)

var validIntensities = map[string]bool{
	"E": true, "M": true, "T": true, "I": true, "R": true, "Rest": true, "": true,
}

// ValidateStructure returns blocking validation errors for a block list.
func ValidateStructure(blocos []domain.BlocoCorrida) []string {
	if len(blocos) == 0 {
		return []string{"lista de blocos não pode ser vazia"}
	}
	var errs []string
	validateRecursive(blocos, 1, &errs)
	return errs
}

func validateRecursive(blocos []domain.BlocoCorrida, depth int, errs *[]string) {
	if depth > 3 {
		*errs = append(*errs, "profundidade de nesting de blocos excede 3")
		return
	}
	for i, b := range blocos {
		prefix := fmt.Sprintf("bloco[%d]", i)
		switch b.Type {
		case "atomic":
			if b.DurationMin <= 0 && b.DistanceKM <= 0 {
				*errs = append(*errs, prefix+": atomic exige duration_min > 0 ou distance_km > 0")
			}
			if !validIntensities[b.Intensity] {
				*errs = append(*errs, prefix+": intensity inválida")
			}
		case "repeater":
			if b.Repeat < 1 {
				*errs = append(*errs, prefix+": repeater exige repeat >= 1")
			}
			if len(b.Content) == 0 {
				*errs = append(*errs, prefix+": repeater exige content não vazio")
			} else {
				validateRecursive(b.Content, depth+1, errs)
			}
		default:
			*errs = append(*errs, prefix+": type deve ser atomic ou repeater")
		}
	}
}

// GenerateWarnings returns non-blocking quality warnings.
func GenerateWarnings(blocos []domain.BlocoCorrida) []string {
	var warnings []string
	if len(blocos) == 0 {
		return warnings
	}
	dur := DurationMinutes(blocos)
	if dur > 150 {
		warnings = append(warnings, "duração total acima de 150 minutos")
	}

	first := blocos[0]
	if first.Type != "atomic" || !strings.EqualFold(first.Intensity, "E") {
		warnings = append(warnings, "sessão sem aquecimento Easy inicial")
	}
	last := blocos[len(blocos)-1]
	if last.Type != "atomic" || !strings.EqualFold(last.Intensity, "E") {
		warnings = append(warnings, "sessão sem volta à calma Easy")
	}

	dist := IntensityDistribution(blocos)
	hard := dist["I"] + dist["R"]
	easy := dist["E"]
	if hard > 0 && easy == 0 {
		warnings = append(warnings, "sessão só com intensidades I/R sem Easy")
	}
	return warnings
}
