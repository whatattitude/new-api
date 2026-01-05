package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Invoice 发票表
type Invoice struct {
	Id         int            `json:"id" gorm:"primaryKey"`
	UserId     int            `json:"user_id" gorm:"index"`                                        // 用户ID
	FileName   string         `json:"file_name" gorm:"type:varchar(255)"`                          // 文件名
	FileData   []byte         `json:"-" gorm:"type:longblob"`                                      // PDF文件数据（不返回给前端）
	FileSize   int64          `json:"file_size"`                                                   // 文件大小（字节）
	MimeType   string         `json:"mime_type" gorm:"type:varchar(50);default:'application/pdf'"` // MIME类型
	Amount     float64        `json:"amount" gorm:"default:0"`                                     // 发票金额（关联订单的支付金额总和）
	CreateTime int64          `json:"create_time" gorm:"bigint"`                                   // 创建时间
	DeletedAt  gorm.DeletedAt `json:"deleted_at" gorm:"index"`                                     // 软删除
}

func (invoice *Invoice) Insert() error {
	return DB.Create(invoice).Error
}

func (invoice *Invoice) Update() error {
	return DB.Save(invoice).Error
}

func GetInvoiceById(id int) *Invoice {
	var invoice Invoice
	if err := DB.Where("id = ?", id).First(&invoice).Error; err != nil {
		return nil
	}
	return &invoice
}

// GetInvoicesByUserId 获取用户的所有发票
func GetInvoicesByUserId(userId int, pageInfo *common.PageInfo) (invoices []*Invoice, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&Invoice{}).Where("user_id = ?", userId).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Where("user_id = ?", userId).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&invoices).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}

// GetAllInvoices 获取所有发票（管理员使用）
func GetAllInvoices(pageInfo *common.PageInfo) (invoices []*Invoice, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&Invoice{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&invoices).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}

// SearchInvoicesByUserId 按用户ID搜索发票
func SearchInvoicesByUserId(userId int, pageInfo *common.PageInfo) (invoices []*Invoice, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&Invoice{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}

	if err = query.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&invoices).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}
