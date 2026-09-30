package decision

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func reportField(value string) string { return strconv.Quote(value) }

func comparisonLabel(state string) string {
	switch state {
	case "no_feasible_options":
		return "没有满足约束的方案。"
	case "no_known_feasible_options":
		return "尚无已知满足约束的方案；存在未确定项。"
	case "preferences_not_provided":
		return "未提供比较偏好，未计算优选集合。"
	case "insufficient_information":
		return "比较所需信息不足，未确定优选集合。"
	case "known_feasible_options_only":
		return "优选集合仅覆盖已知满足约束的方案；若有未确定方案，仍需补充信息。"
	default:
		return "已按提供的偏好比较满足约束的方案。"
	}
}

func statusLabel(state string) string {
	switch state {
	case "feasible":
		return "满足已列约束"
	case "infeasible":
		return "不满足已列约束"
	case "unknown":
		return "尚未确定"
	case "pass":
		return "通过"
	case "fail":
		return "不通过"
	default:
		return state
	}
}

func renderReport(basis result) string {
	var out strings.Builder
	fmt.Fprintf(&out, "决策报告\n参考时间：%s\n快照：%s\n有效至：%s\n", reportField(basis.AsOf), basis.SnapshotID, reportField(basis.SnapshotExpiresAt))
	if basis.ParentSnapshotID != "" {
		fmt.Fprintf(&out, "父版本：%s\n", basis.ParentSnapshotID)
	}
	fmt.Fprintln(&out, "计算基于调用方提供的资料；来源未独立核实，结果仅供参考。")
	fmt.Fprintf(&out, "%s\n优选集合：%s\n不指定唯一推荐；集合内方案可能存在取舍。\n", comparisonLabel(basis.ComparisonState), reportField(strings.Join(basis.ParetoFrontier, ", ")))
	fmt.Fprintln(&out, "\n约束与偏好")
	for _, c := range basis.Constraints {
		fmt.Fprintf(&out, "- %s：%s %s %s %s；依据 %s\n", reportField(c.ID), reportField(c.Metric), reportLabel(c.Op), reportField(c.Limit), reportField(c.Unit), reportField(strings.Join(c.EvidenceIDs, ", ")))
	}
	for _, p := range basis.Preferences {
		fmt.Fprintf(&out, "- 偏好：%s %s，%s\n", reportField(p.Metric), reportField(p.Unit), reportLabel(p.Direction))
	}
	for _, option := range basis.Options {
		renderOption(&out, option)
	}
	fmt.Fprintln(&out, "\n引用资料（来源信息由调用方提供）")
	for _, e := range basis.Evidence {
		fmt.Fprintf(&out, "- %s [%s]：%s；版本 %s；观察时间 %s\n", reportField(e.ID), reportLabel(e.State), reportField(e.Source.URI), reportField(e.Source.Version), reportField(e.ObservedAt))
		if e.ValidUntil != "" {
			fmt.Fprintf(&out, "  资料有效至 %s\n", reportField(e.ValidUntil))
		}
		if len(e.Supersedes) > 0 {
			fmt.Fprintf(&out, "  替代资料 %s\n", reportField(strings.Join(e.Supersedes, ", ")))
		}
	}
	return out.String()
}

func renderOption(out *strings.Builder, option optionResult) {
	fmt.Fprintf(out, "\n方案 %s %s：%s\n", reportField(option.ID), reportField(option.Name), statusLabel(option.Status))
	keys := make([]string, 0, len(option.Metrics))
	for key := range option.Metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		m := option.Metrics[key]
		if m.State == "known" {
			fmt.Fprintf(out, "- %s：%s %s；依据 %s\n", reportField(key), reportField(m.Value), reportField(m.Unit), reportField(strings.Join(m.EvidenceIDs, ", ")))
		} else {
			fmt.Fprintf(out, "- %s：尚未确定；依据 %s\n", reportField(key), reportField(strings.Join(m.EvidenceIDs, ", ")))
			renderIssues(out, &m)
		}
	}
	for _, c := range option.Checks {
		fmt.Fprintf(out, "- 约束 %s：%s；依据 %s\n", reportField(c.ConstraintID), statusLabel(c.Verdict), reportField(strings.Join(c.EvidenceIDs, ", ")))
		renderIssues(out, c.ValueEvidence)
		renderIssues(out, c.LimitEvidence)
	}
}

func renderIssues(out *strings.Builder, m *metricResult) {
	if m == nil {
		return
	}
	for _, issue := range m.Issues {
		fmt.Fprintf(out, "  未确定原因：%s %s\n", reportLabel(issue.Code), reportField(issue.ID))
	}
}
