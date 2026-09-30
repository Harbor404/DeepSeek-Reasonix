package decision

var reportLabels = map[string]string{
	"lte": "不超过", "gte": "不少于", "eq": "等于",
	"min": "越小越好", "max": "越大越好",
	"current": "参考时间内有效", "expired": "已过期", "future": "晚于参考时间",
	"superseded": "已被新资料替代", "conflict": "资料相互冲突",
	"evidence.expired": "资料已过期", "evidence.future": "观察时间晚于参考时间",
	"evidence.superseded": "资料已被替代", "evidence.conflict": "同一事实的资料相互冲突",
	"evidence.missing": "缺少引用资料", "evidence.metric_missing": "缺少所需指标",
	"evidence.metric_mismatch": "资料对应的指标不匹配", "evidence.value_conflict": "引用数值或单位不一致",
	"evidence.unit_mismatch": "约束单位与资料不一致", "evidence.limit_mismatch": "约束数值与引用资料不一致",
}

func reportLabel(code string) string {
	if label, ok := reportLabels[code]; ok {
		return label
	}
	return code
}
