package pan115

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"litepan/internal/domain"
)

func (d *Driver) GetAccountProfile(ctx context.Context) (*domain.AccountProfile, error) {
	var nav struct {
		UserID   flexNumber `json:"user_id"`
		UserName string     `json:"user_name"`
		VipName  string     `json:"vip_name"`
		Vip      flexNumber `json:"vip"`
		VipInfo  struct {
			LevelName string `json:"level_name"`
		} `json:"vip_info"`
	}
	query := url.Values{"_": {nowMillis()}}
	if err := d.apiRequest(ctx, http.MethodGet, myHost+"/?ct=ajax&ac=nav", query, nil, &nav); err != nil {
		return nil, err
	}

	profile := &domain.AccountProfile{
		UserID:     nav.UserID.String(),
		Nickname:   strings.TrimSpace(nav.UserName),
		Membership: membershipName(nav.VipInfo.LevelName, nav.VipName, nav.Vip),
	}

	// 容量信息属补充数据，获取失败不影响资料刷新。
	var info struct {
		SpaceInfo struct {
			AllTotal struct {
				Size flexNumber `json:"size"`
			} `json:"all_total"`
			AllUse struct {
				Size flexNumber `json:"size"`
			} `json:"all_use"`
		} `json:"space_info"`
	}
	if err := d.apiRequest(ctx, http.MethodGet, webAPIHost+"/files/index_info", nil, nil, &info); err == nil {
		profile.TotalBytes = info.SpaceInfo.AllTotal.Size.bytes()
		profile.UsedBytes = info.SpaceInfo.AllUse.Size.bytes()
	}
	return profile, nil
}

func membershipName(levelName, vipName string, vip flexNumber) string {
	if name := firstNonEmpty(levelName, vipName); name != "" && name != "原石会员" {
		return name
	}
	if vip.int64() > 0 {
		return "VIP"
	}
	return ""
}
