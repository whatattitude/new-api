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
  Upload,
  Divider,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Coins, FileText, Search, Upload as UploadIcon, Download } from 'lucide-react';
import {
  API,
  timestamp2string,
  showError,
  showSuccess,
} from '../../helpers';
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

const AdminInvoice = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  
  // 充值记录相关状态
  const [topups, setTopups] = useState([]);
  const [topupsLoading, setTopupsLoading] = useState(false);
  const [topupsPage, setTopupsPage] = useState(1);
  const [topupsPageSize, setTopupsPageSize] = useState(10);
  const [topupsTotal, setTopupsTotal] = useState(0);
  const [topupsKeyword, setTopupsKeyword] = useState('');
  const [selectedTopUpKeys, setSelectedTopUpKeys] = useState([]);
  const [uploadFile, setUploadFile] = useState(null);
  const [uploading, setUploading] = useState(false);

  // 发票列表相关状态
  const [invoices, setInvoices] = useState([]);
  const [invoicesLoading, setInvoicesLoading] = useState(false);
  const [invoicesPage, setInvoicesPage] = useState(1);
  const [invoicesPageSize, setInvoicesPageSize] = useState(10);
  const [invoicesTotal, setInvoicesTotal] = useState(0);
  const [invoicesUserId, setInvoicesUserId] = useState('');

  // 加载充值记录
  const loadTopups = async (currentPage = 1, size = topupsPageSize) => {
    setTopupsLoading(true);
    try {
      const params = {
        page: currentPage,
        page_size: size,
      };
      if (topupsKeyword) {
        params.keyword = topupsKeyword;
      }
      const res = await API.get('/api/user/topup', { params });
      if (res.data.success) {
        const data = res.data.data;
        // 只显示已成功的订单
        const successTopups = (data.items || []).filter(
          (item) => item.status === 'success',
        );
        setTopups(successTopups);
        setTopupsTotal(data.total || 0);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } catch (error) {
      showError(error.message || t('加载失败'));
    } finally {
      setTopupsLoading(false);
    }
  };

  // 加载发票列表
  const loadInvoices = async (currentPage = 1, size = invoicesPageSize) => {
    setInvoicesLoading(true);
    try {
      const params = {
        page: currentPage,
        page_size: size,
      };
      if (invoicesUserId && invoicesUserId.trim() !== '') {
        const userId = parseInt(invoicesUserId);
        if (!isNaN(userId)) {
          params.user_id = userId;
        }
      }
      const res = await API.get('/api/user/invoice/admin', { params });
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

  useEffect(() => {
    loadTopups(topupsPage, topupsPageSize);
  }, [topupsPage, topupsPageSize]);

  useEffect(() => {
    loadInvoices(invoicesPage, invoicesPageSize);
  }, [invoicesPage, invoicesPageSize, invoicesUserId]);

  // 搜索充值记录
  const handleTopupsSearch = () => {
    setTopupsPage(1);
    setSelectedTopUpKeys([]);
    loadTopups(1, topupsPageSize);
  };

  // 搜索发票
  const handleInvoicesSearch = () => {
    setInvoicesPage(1);
    loadInvoices(1, invoicesPageSize);
  };

  // 上传发票
  const handleUploadInvoice = async () => {
    if (selectedTopUpKeys.length === 0) {
      showError(t('请至少选择一个订单'));
      return;
    }

    if (!uploadFile || !uploadFile.file) {
      showError(t('请选择PDF文件'));
      return;
    }

    // 验证文件类型
    const file = uploadFile.file;
    console.log('验证文件:', {
      file,
      type: file?.type,
      name: file?.name,
      isFile: file instanceof File,
    });
    
    // 检查文件类型和扩展名（不区分大小写）
    const fileType = file?.type || '';
    const fileName = file?.name || '';
    const isValidPdf = 
      fileType === 'application/pdf' || 
      (fileName && typeof fileName === 'string' && fileName.toLowerCase().endsWith('.pdf'));
    
    console.log('文件验证结果:', {
      fileType,
      fileName,
      isValidPdf,
    });
    
    if (!isValidPdf) {
      console.error('文件验证失败:', {
        type: fileType,
        name: fileName,
        isValidPdf,
      });
      showError(t('只支持PDF格式文件'));
      return;
    }

    // 验证所选订单是否都是"待开"状态
    const canInvoiceOrders = topups.filter(
      (item) =>
        selectedTopUpKeys.includes(item.id) &&
        (item.invoice_status || '未申请') === '待开',
    );

    if (canInvoiceOrders.length === 0) {
      showError(t('所选订单中没有可以开具发票的订单（必须为"待开"状态）'));
      return;
    }

    if (canInvoiceOrders.length !== selectedTopUpKeys.length) {
      Modal.confirm({
        title: t('确认'),
        content: t('部分订单不是"待开"状态，将只处理符合条件的订单，是否继续？'),
        onOk: () => doUploadInvoice(canInvoiceOrders.map((item) => item.id)),
      });
    } else {
      doUploadInvoice(selectedTopUpKeys);
    }
  };

  const doUploadInvoice = async (orderIds) => {
    if (!uploadFile || !uploadFile.file) {
      showError(t('请选择PDF文件'));
      return;
    }

    setUploading(true);
    try {
      // 计算选中订单的金额总和
      const selectedOrders = topups.filter((item) => orderIds.includes(item.id));
      const totalAmount = selectedOrders.reduce((sum, order) => sum + (order.money || 0), 0);

      const formData = new FormData();
      formData.append('file', uploadFile.file);
      formData.append('top_up_ids', JSON.stringify(orderIds));
      formData.append('amount', totalAmount.toString());

      console.log('上传发票请求:', {
        file: uploadFile.file.name || '未知文件名',
        orderIds,
        amount: totalAmount,
      });

      // axios 会自动处理 FormData，不需要手动设置 Content-Type
      const res = await API.post('/api/user/invoice/admin', formData);

      console.log('上传发票响应:', res.data);

      if (res.data.success) {
        const data = res.data.data;
        showSuccess(
          t('发票创建成功，已关联 {{count}} 个订单，发票ID: {{id}}', {
            count: data.success_count,
            id: data.invoice_id,
          }),
        );
        setUploadFile(null);
        setSelectedTopUpKeys([]);
        await loadTopups(topupsPage, topupsPageSize);
        await loadInvoices(invoicesPage, invoicesPageSize);
      } else {
        showError(res.data.message || t('上传失败'));
      }
    } catch (error) {
      console.error('上传发票错误:', error);
      const errorMessage =
        error.response?.data?.message ||
        error.message ||
        t('上传失败，请检查网络连接或联系管理员');
      showError(errorMessage);
    } finally {
      setUploading(false);
    }
  };

  // 下载发票
  const handleDownloadInvoice = async (invoiceId) => {
    try {
      // 使用 API 请求下载文件，设置 responseType 为 blob
      const res = await API.get(`/api/user/invoice/admin/${invoiceId}/file`, {
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
      
      console.log('下载发票:', {
        invoiceId,
        fileName,
        contentDisposition,
      });
      
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
  const renderPaymentMethod = (pm) => {
    const displayName = PAYMENT_METHOD_MAP[pm];
    return <Text>{displayName ? t(displayName) : pm || '-'}</Text>;
  };

  // 充值记录表格列
  const topupsColumns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        render: (text) => <Text copyable>{text}</Text>,
      },
      {
        title: t('用户ID'),
        dataIndex: 'user_id',
        key: 'user_id',
      },
      {
        title: t('用户名'),
        dataIndex: 'username',
        key: 'username',
        render: (text) => <Text>{text || '-'}</Text>,
      },
      {
        title: t('公司抬头'),
        dataIndex: 'company_name',
        key: 'company_name',
        render: (text) => <Text>{text || '-'}</Text>,
      },
      {
        title: t('税号'),
        dataIndex: 'tax_number',
        key: 'tax_number',
        render: (text) => <Text>{text || '-'}</Text>,
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
            <Text>{amount}</Text>
          </span>
        ),
      },
      {
        title: t('支付金额'),
        dataIndex: 'money',
        key: 'money',
        render: (money) => <Text type='danger'>¥{money.toFixed(2)}</Text>,
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
        render: (id) => (id ? <Text>{id}</Text> : <Text type='tertiary'>-</Text>),
      },
      {
        title: t('创建时间'),
        dataIndex: 'create_time',
        key: 'create_time',
        render: (time) => timestamp2string(time),
      },
    ],
    [t],
  );

  // 发票列表表格列
  const invoicesColumns = useMemo(
    () => [
      {
        title: t('发票ID'),
        dataIndex: 'id',
        key: 'id',
      },
      {
        title: t('用户ID'),
        dataIndex: 'user_id',
        key: 'user_id',
      },
      {
        title: t('文件名'),
        dataIndex: 'file_name',
        key: 'file_name',
        render: (name) => <Text>{name}</Text>,
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
        render: (time) => timestamp2string(time),
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
    ],
    [t],
  );

  // 充值记录行选择配置
  const topupsRowSelection = {
    selectedRowKeys: selectedTopUpKeys,
    onChange: (selectedKeys) => {
      setSelectedTopUpKeys(selectedKeys);
    },
    getCheckboxProps: (record) => ({
      disabled: (record.invoice_status || '未申请') !== '待开',
    }),
  };

  return (
    <div className='mt-[60px] px-2 max-w-7xl mx-auto'>
      {/* 充值记录部分 */}
      <Card className='mb-4'>
        <div className='flex flex-col gap-4'>
          <Title heading={4}>{t('充值记录管理')}</Title>
          <div className='flex gap-2 flex-wrap'>
            <Input
              prefix={<Search size={16} />}
              placeholder={t('搜索订单号')}
              value={topupsKeyword}
              onChange={setTopupsKeyword}
              onEnterPress={handleTopupsSearch}
              showClear
              style={{ flex: 1, minWidth: 200 }}
            />
            <Button type='primary' onClick={handleTopupsSearch}>
              {t('搜索')}
            </Button>
          </div>
          {selectedTopUpKeys.length > 0 && (
            <div className='flex gap-2 items-center flex-wrap'>
              <Text>
                {t('已选择 {{count}} 个订单', { count: selectedTopUpKeys.length })}
              </Text>
              <Text type='danger' strong>
                {t('合计金额: ¥{{amount}}', {
                  amount: topups
                    .filter((item) => selectedTopUpKeys.includes(item.id))
                    .reduce((sum, order) => sum + (order.money || 0), 0)
                    .toFixed(2),
                })}
              </Text>
              <Upload
                accept='.pdf,application/pdf'
                showUploadList={false}
                beforeUpload={(file) => {
                  console.log('选择文件:', file);
                  // Semi-UI Upload 可能传递 File 对象或包装对象
                  // 根据日志，实际结构可能是 {file: {fileInstance: File}}
                  let actualFile = null;
                  
                  if (file instanceof File) {
                    // 直接是 File 对象
                    actualFile = file;
                  } else if (file && typeof file === 'object') {
                    // 尝试从不同路径提取 File 对象
                    actualFile = file.fileInstance || 
                                file.file?.fileInstance || 
                                file.file ||
                                file;
                  }
                  
                  console.log('提取的文件对象:', actualFile);
                  
                  if (actualFile && actualFile instanceof File) {
                    setUploadFile({ file: actualFile });
                    console.log('文件设置成功:', actualFile.name, actualFile.type);
                  } else {
                    console.error('无效的文件对象:', file, actualFile);
                    showError(t('文件选择失败，请重试'));
                  }
                  return false; // 阻止自动上传
                }}
                onRemove={() => {
                  console.log('移除文件');
                  setUploadFile(null);
                }}
              >
                <Button icon={<UploadIcon size={16} />}>
                  {uploadFile && uploadFile.file && uploadFile.file.name
                    ? uploadFile.file.name
                    : t('选择PDF文件')}
                </Button>
              </Upload>
              <Button
                type='primary'
                theme='solid'
                icon={<FileText size={16} />}
                loading={uploading}
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  console.log('按钮点击，uploadFile:', uploadFile);
                  handleUploadInvoice();
                }}
                disabled={!uploadFile || !uploadFile.file}
              >
                {t('上传并关联发票')}
              </Button>
            </div>
          )}
        </div>
      </Card>

      <Table
        columns={topupsColumns}
        dataSource={topups}
        loading={topupsLoading}
        rowKey='id'
        rowSelection={topupsRowSelection}
        pagination={{
          currentPage: topupsPage,
          pageSize: topupsPageSize,
          total: topupsTotal,
          onPageChange: (newPage) => {
            setTopupsPage(newPage);
            setSelectedTopUpKeys([]);
          },
          onPageSizeChange: (newSize) => {
            setTopupsPageSize(newSize);
            setTopupsPage(1);
            setSelectedTopUpKeys([]);
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
            title={t('暂无充值记录')}
            description={t('还没有任何已完成的充值订单')}
          />
        }
      />

      <Divider margin={24} />

      {/* 已开发票列表部分 */}
      <Card className='mb-4'>
        <div className='flex flex-col gap-4'>
          <Title heading={4}>{t('已开发票列表')}</Title>
          <div className='flex gap-2'>
            <Input
              prefix={<Search size={16} />}
              placeholder={t('搜索用户ID')}
              value={invoicesUserId}
              onChange={setInvoicesUserId}
              onEnterPress={handleInvoicesSearch}
              showClear
              style={{ flex: 1 }}
            />
            <Button type='primary' onClick={handleInvoicesSearch}>
              {t('搜索')}
            </Button>
          </div>
        </div>
      </Card>

      <Table
        columns={invoicesColumns}
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

export default AdminInvoice;

