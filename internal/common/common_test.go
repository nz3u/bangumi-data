package common

import "testing"

func TestLoad(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// 作品类型
	if got := c.SubjectTypeCN(2); got != "动画" {
		t.Errorf("SubjectTypeCN(2) = %q, want 动画", got)
	}
	// 平台：动画 -> TV
	if got := c.PlatformCN(2, 1); got != "TV" {
		t.Errorf("PlatformCN(2,1) = %q, want TV", got)
	}
	// 漫画 -> 漫画
	if got := c.PlatformCN(1, 1001); got != "漫画" {
		t.Errorf("PlatformCN(1,1001) = %q, want 漫画", got)
	}
	// 游戏平台兜底
	if got := c.PlatformCN(4, 4); got != "PC" {
		t.Errorf("PlatformCN(4,4) = %q, want PC", got)
	}
	// 条目关联：动画 2 -> 前传
	if got := c.RelationCN(2, 2); got != "前传" {
		t.Errorf("RelationCN(2,2) = %q, want 前传", got)
	}
	// 职位：动画 2 -> 导演
	if got := c.StaffCN(2, 2); got != "导演" {
		t.Errorf("StaffCN(2,2) = %q, want 导演", got)
	}
	// 人物关联：crt 2006 -> 亲属
	if got := c.PersonRelationCN("crt", 2006); got != "亲属" {
		t.Errorf("PersonRelationCN(crt,2006) = %q, want 亲属", got)
	}
	// 人物关联：prsn 1004 -> 创始人
	if got := c.PersonRelationCN("prsn", 1004); got != "创始人" {
		t.Errorf("PersonRelationCN(prsn,1004) = %q, want 创始人", got)
	}
	// 章节类型
	if got := c.EpisodeTypes[2]; got != "OP" {
		t.Errorf("EpisodeTypes[2] = %q, want OP", got)
	}
	// 职位 categories 解析（动画职位 20 原画 应带 categories）
	if m, ok := c.Staffs[2]; ok {
		if s, ok2 := m[20]; ok2 {
			if len(s.Categories) == 0 {
				t.Errorf("Staff(2,20) categories 为空")
			}
		} else {
			t.Errorf("Staffs[2][20] 不存在")
		}
	}
}
