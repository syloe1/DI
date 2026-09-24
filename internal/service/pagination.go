package service

// normalizePagination 归一化分页参数：非法值（<=0）回落到默认值。
func normalizePagination(page int, pageSize int, defaultPage int, defaultPageSize int) (int, int) {
	if page <= 0 {
		page = defaultPage
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return page, pageSize
}
