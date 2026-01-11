/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Table,
  Badge,
  Typography,
  Toast,
  Empty,
  Button,
  Input,
  Modal,
  Card,
  Divider,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Coins, FileText, Search, Download } from 'lucide-react';
import { API, timestamp2string, showError, showSuccess } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

// 发票状态映射配置
const INVOICE_STATUS_CONFIG = {
  未申请: { type: 'default', text: '未申请' },
  待开: { type: 'warning', text: '待开' },
  已发送: { type: 'success', text: '已发送' },
};

// 订单状态映射配置
const ORDER_STATUS_CONFIG = {
  success: { type: 'success', text: '成功' },
  pending: { type: 'warning', text: '待支付' },
  expired: { type: 'danger', text: '已过期' },
};

// 支付方式映射
const PAYMENT_METHOD_MAP = {
  stripe: 'Stripe',
  alipay: '支付宝',
  wxpay: '微信',
};

const Invoice = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const isMobile = useIsMobile();
  const [topups, setTopups] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [total, setTotal] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [batchLoading, setBatchLoading] = useState(false);
  const [userInfo, setUserInfo] = useState(null);

  // 发票列表相关状态
  const [invoices, setInvoices] = useState([]);
  const [invoicesLoading, setInvoicesLoading] = useState(false);
  const [invoicesPage, setInvoicesPage] = useState(1);
  const [invoicesPageSize, setInvoicesPageSize] = useState(10);
  const [invoicesTotal, setInvoicesTotal] = useState(0);

  // 加载发票列表
  const loadInvoices = async (currentPage = 1, size = pageSize) => {
    setLoading(true);
    try {
      const params = {
        page: currentPage,
        page_size: size,
      };
      if (keyword) {
        params.keyword = keyword;
      }
      const res = await API.get('/api/user/topup/self', { params });
      if (res.data.success) {
        const data = res.data.data;
        // 只显示已成功的订单
        const successTopups = (data.items || []).filter(
          (item) => item.status === 'success',
        );
        setTopups(successTopups);
        setTotal(data.total || 0);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } catch (error) {
      showError(error.message || t('加载失败'));
    } finally {
      setLoading(false);
    }
  };

  // 加载发票列表
  const loadUserInvoices = async (currentPage = 1, size = invoicesPageSize) => {
    setInvoicesLoading(true);
    try {
      const params = {
        page: currentPage,
        page_size: size,
      };
      const res = await API.get('/api/user/invoice', { params });
      if (res.data.success) {
        const data = res.data.data;
        setInvoices(data.items || []);
        setInvoicesTotal(data.total || 0);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } catch (error) {
      showError(error.message || t('加载失败'));
    } finally {
      setInvoicesLoading(false);
    }
  };

  // 加载用户信息以获取倍率
  const loadUserInfo = async () => {
    try {
      const res = await API.get('/api/user/self');
      if (res.data.success) {
        setUserInfo(res.data.data);
      }
    } catch (error) {
      console.error('加载用户信息失败:', error);
    }
  };

  useEffect(() => {
    loadInvoices(page, pageSize);
    loadUserInfo();
  }, [page, pageSize]);

  useEffect(() => {
    loadUserInvoices(invoicesPage, invoicesPageSize);
  }, [invoicesPage, invoicesPageSize]);

  // 搜索处理
  const handleSearch = () => {
    setPage(1);
    setSelectedRowKeys([]); // 清空选中项
    loadInvoices(1, pageSize);
  };

  const handleKeywordChange = (value) => {
    setKeyword(value);
  };

  // 申请发票
  const handleApplyInvoice = async (topup) => {
    try {
      const res = await API.put('/api/user/topup/invoice', {
        id: topup.id,
        invoice_status: '待开',
      });
      if (res.data.success) {
        showSuccess(t('发票申请成功'));
        await loadInvoices(page, pageSize);
      } else {
        const errorMessage = res.data.message || t('申请失败');
        // 检查是否是发票信息未填写的错误
        if (errorMessage.includes('发票信息未填写')) {
          Modal.error({
            title: t('发票信息未填写'),
            content: (
              <div>
                <p>{t('请先完善发票信息（公司抬头和税号）后再申请发票')}</p>
                <Button
                  type='primary'
                  theme='solid'
                  onClick={() => {
                    navigate('/console/personal');
                  }}
                  style={{ marginTop: '12px' }}
                >
                  {t('前往个人设置')}
                </Button>
              </div>
            ),
          });
        } else {
          showError(errorMessage);
        }
      }
    } catch (error) {
      const errorMessage = error.response?.data?.message || error.message || t('申请失败');
      // 检查是否是发票信息未填写的错误
      if (errorMessage.includes('发票信息未填写')) {
        Modal.error({
          title: t('发票信息未填写'),
          content: (
            <div>
              <p>{t('请先完善发票信息（公司抬头和税号）后再申请发票')}</p>
              <Button
                type='primary'
                theme='solid'
                onClick={() => {
                  navigate('/console/personal');
                }}
                style={{ marginTop: '12px' }}
              >
                {t('前往个人设置')}
              </Button>
            </div>
          ),
        });
      } else {
        showError(errorMessage);
      }
    }
  };

  // 确认申请发票
  const confirmApplyInvoice = (topup) => {
    const companyName = userInfo?.company_name || '';
    const taxNumber = userInfo?.tax_number || '';
    const hasInvoiceInfo = companyName && taxNumber;
    
    Modal.confirm({
      title: t('确认申请发票'),
      content: (
        <div>
          <p style={{ marginBottom: '12px' }}>
            {t('确定要为订单 {{tradeNo}} 申请发票吗？', {
              tradeNo: topup.trade_no,
            })}
          </p>
          {hasInvoiceInfo ? (
            <div style={{ 
              marginTop: '16px', 
              padding: '12px', 
              backgroundColor: 'var(--semi-color-fill-0)', 
              borderRadius: '4px' 
            }}>
              <p style={{ marginBottom: '8px', fontWeight: '500' }}>
                {t('发票信息')}:
              </p>
              <p style={{ marginBottom: '4px' }}>
                <span style={{ color: 'var(--semi-color-text-2)' }}>
                  {t('公司抬头')}:
                </span>{' '}
                <span>{companyName}</span>
              </p>
              <p>
                <span style={{ color: 'var(--semi-color-text-2)' }}>
                  {t('税号')}:
                </span>{' '}
                <span>{taxNumber}</span>
              </p>
            </div>
          ) : (
            <div style={{ 
              marginTop: '16px', 
              padding: '12px', 
              backgroundColor: 'var(--semi-color-warning-light-active)', 
              borderRadius: '4px' 
            }}>
              <p style={{ marginBottom: '8px', color: 'var(--semi-color-warning)' }}>
                {t('提示：发票信息未填写')}
              </p>
              <Button
                type='primary'
                theme='solid'
                size='small'
                onClick={() => {
                  Modal.destroyAll();
                  navigate('/console/personal');
                }}
              >
                {t('前往个人设置')}
              </Button>
            </div>
          )}
        </div>
      ),
      onOk: () => handleApplyInvoice(topup),
      width: 480,
    });
  };

  // 批量申请发票
  const handleBatchApplyInvoice = async () => {
    if (selectedRowKeys.length === 0) {
      showError(t('请至少选择一个订单'));
      return;
    }

    // 过滤出可以申请的订单（状态为"未申请"）
    const canApplyOrders = topups.filter(
      (item) =>
        selectedRowKeys.includes(item.id) &&
        (item.invoice_status || '未申请') === '未申请',
    );

    if (canApplyOrders.length === 0) {
      showError(t('所选订单中没有可以申请发票的订单'));
      return;
    }

    Modal.confirm({
      title: t('确认批量申请发票'),
      content: t('确定要为 {{count}} 个订单批量申请发票吗？', {
        count: canApplyOrders.length,
      }),
      onOk: async () => {
        setBatchLoading(true);
        try {
          const res = await API.put('/api/user/topup/invoice/batch', {
            ids: canApplyOrders.map((item) => item.id),
            invoice_status: '待开',
          });
          if (res.data.success) {
            const data = res.data.data;
            if (data.failed_count > 0) {
              showSuccess(
                t('成功申请 {{success}} 个订单，失败 {{failed}} 个订单', {
                  success: data.success_count,
                  failed: data.failed_count,
                }),
              );
            } else {
              showSuccess(
                t('成功为 {{count}} 个订单申请发票', {
                  count: data.success_count,
                }),
              );
            }
            setSelectedRowKeys([]);
            await loadInvoices(page, pageSize);
          } else {
            const errorMessage = res.data.message || t('批量申请失败');
            // 检查是否是发票信息未填写的错误
            if (errorMessage.includes('发票信息未填写')) {
              Modal.error({
                title: t('发票信息未填写'),
                content: (
                  <div>
                    <p>{t('请先完善发票信息（公司抬头和税号）后再申请发票')}</p>
                    <Button
                      type='primary'
                      theme='solid'
                      onClick={() => {
                        navigate('/console/personal');
                      }}
                      style={{ marginTop: '12px' }}
                    >
                      {t('前往个人设置')}
                    </Button>
                  </div>
                ),
              });
            } else {
              showError(errorMessage);
            }
          }
        } catch (error) {
          const errorMessage = error.response?.data?.message || error.message || t('批量申请失败');
          // 检查是否是发票信息未填写的错误
          if (errorMessage.includes('发票信息未填写')) {
            Modal.error({
              title: t('发票信息未填写'),
              content: (
                <div>
                  <p>{t('请先完善发票信息（公司抬头和税号）后再申请发票')}</p>
                  <Button
                    type='primary'
                    theme='solid'
                    onClick={() => {
                      navigate('/console/personal');
                    }}
                    style={{ marginTop: '12px' }}
                  >
                    {t('前往个人设置')}
                  </Button>
                </div>
              ),
            });
          } else {
            showError(errorMessage);
          }
        } finally {
          setBatchLoading(false);
        }
      },
    });
  };

  // 行选择配置
  const rowSelection = {
    selectedRowKeys,
    onChange: (selectedKeys) => {
      setSelectedRowKeys(selectedKeys);
    },
    getCheckboxProps: (record) => ({
      disabled: (record.invoice_status || '未申请') !== '未申请',
    }),
  };

  // 渲染发票状态徽章
  const renderInvoiceStatusBadge = (status) => {
    const config =
      INVOICE_STATUS_CONFIG[status] || { type: 'default', text: status };
    return (
      <span className='flex items-center gap-2'>
        <Badge dot type={config.type} />
        <span>{t(config.text)}</span>
      </span>
    );
  };

  // 渲染订单状态徽章
  const renderOrderStatusBadge = (status) => {
    const config =
      ORDER_STATUS_CONFIG[status] || { type: 'primary', text: status };
    return (
      <span className='flex items-center gap-2'>
        <Badge dot type={config.type} />
        <span>{t(config.text)}</span>
      </span>
    );
  };

  // 渲染支付方式
  const renderPaymentMethod = (pm, record) => {
    if (!pm && !record) {
      return <Text>-</Text>;
    }
    const paymentMethod = pm || record?.payment_method || '-';
    const displayName = PAYMENT_METHOD_MAP[paymentMethod];
    return <Text>{displayName ? t(displayName) : paymentMethod}</Text>;
  };

  // 下载发票
  const handleDownloadInvoice = async (invoiceId) => {
    try {
      // 使用 API 请求下载文件，设置 responseType 为 blob
      const res = await API.get(`/api/user/invoice/${invoiceId}/file`, {
        responseType: 'blob',
      });

      // 创建 blob URL
      const blob = new Blob([res.data], { type: 'application/pdf' });
      const url = window.URL.createObjectURL(blob);
      
      // 创建临时链接并触发下载
      const link = document.createElement('a');
      link.href = url;
      
      // 从响应头获取文件名，如果没有则使用默认名称
      // axios 可能将响应头转换为小写，所以需要检查两种格式
      const contentDisposition = 
        res.headers['content-disposition'] || 
        res.headers['Content-Disposition'];
      let fileName = `invoice_${invoiceId}.pdf`;
      
      if (contentDisposition) {
        // 优先匹配 RFC 5987 格式的 filename* (UTF-8 编码)
        const filenameStarMatch = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i);
        if (filenameStarMatch && filenameStarMatch[1]) {
          try {
            // 解码 URL 编码的文件名
            fileName = decodeURIComponent(filenameStarMatch[1]);
          } catch (e) {
            console.warn('文件名解码失败:', e);
            // 如果解码失败，尝试匹配普通的 filename
            const fileNameMatch = contentDisposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/i);
            if (fileNameMatch && fileNameMatch[1]) {
              fileName = fileNameMatch[1].replace(/['"]/g, '');
            }
          }
        } else {
          // 如果没有 filename*，尝试匹配普通的 filename
          const fileNameMatch = contentDisposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/i);
          if (fileNameMatch && fileNameMatch[1]) {
            // 移除引号
            fileName = fileNameMatch[1].replace(/['"]/g, '');
            // 尝试解码 URI（兼容旧格式）
            try {
              fileName = decodeURIComponent(fileName);
            } catch (e) {
              // 如果解码失败，使用原始值
              console.warn('文件名解码失败:', e);
            }
          }
        }
      }
      
      link.download = fileName;
      document.body.appendChild(link);
      link.click();
      
      // 清理
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
      
      showSuccess(t('发票下载成功'));
    } catch (error) {
      console.error('下载发票错误:', error);
      const errorMessage =
        error.response?.data?.message ||
        error.message ||
        t('下载失败，请检查网络连接或联系管理员');
      showError(errorMessage);
    }
  };

  const columns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        render: (value) => <Text copyable>{value != null ? value : '-'}</Text>,
      },
      {
        title: t('支付方式'),
        dataIndex: 'payment_method',
        key: 'payment_method',
        render: renderPaymentMethod,
      },
      {
        title: t('充值额度'),
        dataIndex: 'amount',
        key: 'amount',
        render: (amount) => (
          <span className='flex items-center gap-1'>
            <Coins size={16} />
            <Text>{amount != null ? amount : '-'}</Text>
          </span>
        ),
      },
      {
        title: t('支付金额'),
        dataIndex: 'money',
        key: 'money',
        render: (money) => <Text type='danger'>¥{money != null ? money.toFixed(2) : '0.00'}</Text>,
      },
      {
        title: t('赠送金额'),
        key: 'bonus',
        render: (_, record) => {
          if (!userInfo || !userInfo.topup_multiplier) {
            return <Text type='tertiary'>-</Text>;
          }
          const multiplier = userInfo.topup_multiplier || 1.0;
          const money = record?.money ?? 0;
          const bonusAmount = money * (multiplier - 1);
          if (bonusAmount > 0) {
            return <Text type='success'>¥{bonusAmount.toFixed(2)}</Text>;
          }
          return <Text type='tertiary'>¥0.00</Text>;
        },
      },
      {
        title: t('实际到账'),
        key: 'actual',
        render: (_, record) => {
          // 优先使用后端返回的 actual_money，如果没有则使用 money * multiplier 计算
          const actualAmount = record?.actual_money ?? (record?.money ?? 0) * (userInfo?.topup_multiplier || 1.0);
          return <Text type='success' strong>¥{actualAmount.toFixed(2)}</Text>;
        },
      },
      {
        title: t('订单状态'),
        dataIndex: 'status',
        key: 'status',
        render: renderOrderStatusBadge,
      },
      {
        title: t('发票状态'),
        dataIndex: 'invoice_status',
        key: 'invoice_status',
        render: (status) => renderInvoiceStatusBadge(status || '未申请'),
      },
      {
        title: t('发票ID'),
        dataIndex: 'invoice_id',
        key: 'invoice_id',
        render: (id) => (id != null ? <Text>{id}</Text> : <Text type='tertiary'>-</Text>),
      },
      {
        title: t('操作'),
        key: 'action',
        render: (_, record) => {
          const invoiceStatus = record.invoice_status || '未申请';
          if (invoiceStatus === '未申请') {
            return (
              <Button
                size='small'
                type='primary'
                theme='solid'
                icon={<FileText size={14} />}
                onClick={() => confirmApplyInvoice(record)}
              >
                {t('申请发票')}
              </Button>
            );
          } else if (invoiceStatus === '已发送' && record.invoice_id) {
            return (
              <Button
                size='small'
                type='primary'
                theme='outline'
                icon={<Download size={14} />}
                onClick={() => handleDownloadInvoice(record.invoice_id)}
              >
                {t('下载发票')}
              </Button>
            );
          }
          return <Text type='tertiary'>{t('已申请')}</Text>;
        },
      },
      {
        title: t('创建时间'),
        dataIndex: 'create_time',
        key: 'create_time',
        render: (time) => <Text>{timestamp2string(time)}</Text>,
      },
    ],
    [t, userInfo],
  );

  return (
    <div className='mt-[60px] px-2 max-w-7xl mx-auto'>
      <Card className='mb-4'>
        <div className='flex flex-col gap-4'>
          <div className='flex items-center justify-between'>
            <Title heading={4}>{t('发票管理')}</Title>
            <div className='flex items-center gap-2'>
              <Button
                type='tertiary'
                theme='borderless'
                icon={<FileText size={16} />}
                onClick={() => navigate('/console/personal')}
              >
                {t('前往个人设置')}
              </Button>
              {selectedRowKeys.length > 0 && (
                <Button
                  type='primary'
                  theme='solid'
                  icon={<FileText size={16} />}
                  loading={batchLoading}
                  onClick={handleBatchApplyInvoice}
                >
                  {t('批量申请发票 ({{count}})', { count: selectedRowKeys.length })}
                </Button>
              )}
            </div>
          </div>
          <div className='flex gap-2'>
            <Input
              prefix={<Search size={16} />}
              placeholder={t('搜索订单号')}
              value={keyword}
              onChange={handleKeywordChange}
              onEnterPress={handleSearch}
              showClear
              style={{ flex: 1 }}
            />
            <Button type='primary' onClick={handleSearch}>
              {t('搜索')}
            </Button>
          </div>
        </div>
      </Card>

      <Table
        columns={columns}
        dataSource={topups}
        loading={loading}
        rowKey='id'
        rowSelection={rowSelection}
        pagination={{
          currentPage: page,
          pageSize: pageSize,
          total: total,
          onPageChange: (newPage) => {
            setPage(newPage);
            setSelectedRowKeys([]); // 切换页面时清空选中项
          },
          onPageSizeChange: (newSize) => {
            setPageSize(newSize);
            setPage(1);
            setSelectedRowKeys([]); // 切换页面大小时清空选中项
          },
          showSizeChanger: true,
          showQuickJumper: true,
        }}
        empty={
          <Empty
            image={
              <IllustrationNoResult
                style={{ width: 150, height: 150 }}
              />
            }
            darkModeImage={
              <IllustrationNoResultDark
                style={{ width: 150, height: 150 }}
              />
            }
            title={t('暂无发票记录')}
            description={t('您还没有任何已完成的充值订单')}
          />
        }
      />

      <Divider margin={24} />

      {/* 已开发票列表部分 */}
      <Card className='mb-4'>
        <div className='flex flex-col gap-4'>
          <Title heading={4}>{t('已开发票列表')}</Title>
        </div>
      </Card>

      <Table
        columns={[
          {
            title: t('发票ID'),
            dataIndex: 'id',
            key: 'id',
            render: (id) => <Text>{id != null ? id : '-'}</Text>,
          },
          {
            title: t('文件名'),
            dataIndex: 'file_name',
            key: 'file_name',
            render: (name) => <Text>{name != null ? name : '-'}</Text>,
          },
          {
            title: t('发票金额'),
            dataIndex: 'amount',
            key: 'amount',
            render: (amount) => (
              <Text type='danger' strong>¥{amount ? amount.toFixed(2) : '0.00'}</Text>
            ),
          },
          {
            title: t('文件大小'),
            dataIndex: 'file_size',
            key: 'file_size',
            render: (size) => {
              const mb = (size / 1024 / 1024).toFixed(2);
              return <Text>{mb} MB</Text>;
            },
          },
          {
            title: t('创建时间'),
            dataIndex: 'create_time',
            key: 'create_time',
            render: (time) => <Text>{timestamp2string(time)}</Text>,
          },
          {
            title: t('操作'),
            key: 'action',
            render: (_, record) => (
              <Button
                size='small'
                type='primary'
                theme='outline'
                icon={<Download size={14} />}
                onClick={() => handleDownloadInvoice(record.id)}
              >
                {t('下载')}
              </Button>
            ),
          },
        ]}
        dataSource={invoices}
        loading={invoicesLoading}
        rowKey='id'
        pagination={{
          currentPage: invoicesPage,
          pageSize: invoicesPageSize,
          total: invoicesTotal,
          onPageChange: (newPage) => {
            setInvoicesPage(newPage);
          },
          onPageSizeChange: (newSize) => {
            setInvoicesPageSize(newSize);
            setInvoicesPage(1);
          },
          showSizeChanger: true,
          showQuickJumper: true,
        }}
        empty={
          <Empty
            image={
              <IllustrationNoResult style={{ width: 150, height: 150 }} />
            }
            darkModeImage={
              <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
            }
            title={t('暂无发票记录')}
            description={t('还没有任何已开发的发票')}
          />
        }
      />
    </div>
  );
};

export default Invoice;

