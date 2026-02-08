# Xray设置Slave隔离安全分析报告

## 执行日期
2026-02-06

## 分析范围
检查xray设置页面的所有方法和函数，确认是否正确关联到slave，是否存在配置泄漏风险。

---

## 🔴 严重问题 - 发现配置泄漏风险

### 问题1: **Xray模板配置（xrayTemplateConfig）全局共享**

#### 问题描述
xray模板配置存储在全局setting表中，不区分slave。所有slave共享同一份配置模板。

#### 受影响的API
1. **GET `/panel/xray/`** (getXraySetting) - 获取xray设置
   - 返回全局的`xrayTemplateConfig`
   - 没有任何slave隔离

2. **POST `/panel/xray/update`** (updateSetting) - 更新xray设置  
   - 直接保存到全局配置
   - 修改会影响所有slave

#### 代码位置
- 前端: `web/html/xray.html` 第382-404行
  ```javascript
  async getXraySetting() {
    const msg = await HttpUtil.post("/panel/xray/");  // ❌ 无slaveId参数
    ...
  }
  
  async updateXraySetting() {
    const msg = await HttpUtil.post("/panel/xray/update", 
      { xraySetting: this.xraySetting });  // ❌ 无slaveId参数
    ...
  }
  ```

- 后端: `web/controller/xray_setting.go` 第42-62行
  ```go
  func (a *XraySettingController) getXraySetting(c *gin.Context) {
      xraySetting, err := a.SettingService.GetXrayConfigTemplate()
      // ❌ 从全局setting表获取，不区分slave
      ...
  }
  
  func (a *XraySettingController) updateSetting(c *gin.Context) {
      xraySetting := c.PostForm("xraySetting")
      err := a.XraySettingService.SaveXraySetting(xraySetting)
      // ❌ 保存到全局setting表，影响所有slave
      ...
  }
  ```

- 存储层: `web/service/setting.go` 第270-272行
  ```go
  func (s *SettingService) GetXrayConfigTemplate() (string, error) {
      return s.getString("xrayTemplateConfig")
      // ❌ 从全局setting表读取
  }
  ```

#### 泄漏风险评估
**风险等级: 🔴 高**

1. **配置串改**: 对Slave A的xray配置修改会影响Slave B
2. **安全隔离失效**: 无法为不同slave设置不同的xray基础配置（如日志级别、DNS、路由策略等）
3. **运维风险**: 管理员可能误以为修改只影响当前slave，实际影响全局

#### 影响场景示例
```
1. 管理员打开 Slave A 的xray设置页面（带slaveId参数）
2. 修改日志级别从 warning 改为 debug
3. 点击保存
4. 结果: 所有slave的日志级别都变成debug，包括Slave B、C、D...
```

---

### 问题2: **Outbound流量统计未区分Slave**

#### 问题描述
`getOutboundsTraffic` API返回所有slave的outbound流量数据，未根据页面的slaveId参数过滤。

#### 受影响的API
- **GET `/panel/xray/getOutboundsTraffic`**

#### 代码位置
- 前端: `web/html/xray.html` 第376-380行
  ```javascript
  async getOutboundsTraffic() {
    const msg = await HttpUtil.get("/panel/xray/getOutboundsTraffic");
    // ❌ 未传递slaveId参数
    if (msg.success) {
      this.outboundsTraffic = msg.obj;
    }
  }
  ```

- 后端: `web/controller/xray_setting.go` 第107-114行
  ```go
  func (a *XraySettingController) getOutboundsTraffic(c *gin.Context) {
      outboundsTraffic, err := a.OutboundService.GetOutboundsTraffic()
      // ❌ 获取所有outbound流量，不区分slave
      ...
  }
  ```

#### 泄漏风险评估
**风险等级: 🟡 中**

1. **数据泄漏**: Slave A的管理员可能看到Slave B的流量数据
2. **混淆风险**: 流量统计显示不准确，影响决策

---

### 问题3: **Reset Outbound流量未区分Slave**

#### 问题描述
重置outbound流量时未指定slaveId，可能影响多个slave的同名outbound。

#### 受影响的API
- **POST `/panel/xray/resetOutboundsTraffic`**

#### 代码位置
- 前端: `web/html/xray.html` 第776-784行
  ```javascript
  async resetOutboundTraffic(index) {
    let tag = "-alltags-";
    if (index >= 0) {
      tag = this.outboundData[index].tag ? this.outboundData[index].tag : ""
    }
    const msg = await HttpUtil.post("/panel/xray/resetOutboundsTraffic", 
      { tag: tag });  // ❌ 只传tag，没有slaveId
    ...
  }
  ```

- 后端: `web/controller/xray_setting.go` 第116-124行
  ```go
  func (a *XraySettingController) resetOutboundsTraffic(c *gin.Context) {
      tag := c.PostForm("tag")
      err := a.OutboundService.ResetOutboundTraffic(tag)
      // ❌ 按tag重置，可能影响多个slave的同名outbound
      ...
  }
  ```

#### 泄漏风险评估
**风险等级: 🟡 中**

1. **误操作风险**: 重置Slave A的outbound可能同时重置Slave B的同名outbound
2. **数据一致性**: 流量统计可能出现异常

---

## ✅ 已正确实现Slave隔离的API

### 1. Inbound管理
✅ **GET `/panel/api/inbounds/list?slaveId=X`**
- 前端传递slaveId参数
- 后端根据slaveId过滤
- 代码: `web/controller/inbound.go` 第60-76行

### 2. Outbound管理
✅ **GET `/panel/api/outbounds/list?slaveId=X`**
- 前端传递slaveId参数
- 后端根据slaveId过滤
- Outbound模型包含SlaveId字段
- 代码: `web/controller/xray_outbound.go` 第28-48行

✅ **POST `/panel/api/outbounds/add`**
- 前端表单包含slaveId
- 后端验证slaveId存在性
- 保存后自动push配置到对应slave
- 代码: `web/controller/xray_outbound.go` 第50-64行

✅ **POST `/panel/api/outbounds/update`**
- 更新时保持原slaveId不变
- 自动push配置到对应slave

✅ **POST `/panel/api/outbounds/del/:id`**
- 通过id删除，id已关联slaveId

### 3. Routing Rule管理
✅ **GET `/panel/api/routing/list?slaveId=X`**
- 前端传递slaveId参数
- 后端根据slaveId过滤
- 代码: `web/controller/xray_routing.go` 第28-48行

✅ **POST `/panel/api/routing/add`**
- RoutingRule模型包含SlaveId字段
- 保存后自动push配置到对应slave

✅ **POST `/panel/api/routing/update`**
- 更新时保持原slaveId
- 自动push配置到对应slave

✅ **POST `/panel/api/routing/del/:id`**
- 通过id删除，id已关联slaveId

### 4. Xray服务重启
✅ **POST `/panel/api/server/restartSlaveXray/:slaveId`**
- 正确指定slaveId
- 只重启对应slave的xray服务
- 代码: `web/html/xray.html` 第407-422行

---

## 🔧 修复建议

### 优先级1 (高) - Xray模板配置隔离

#### 方案A: 为每个Slave单独存储配置模板 (推荐)

**数据库结构变更:**
```sql
-- 新建slave_settings表
CREATE TABLE slave_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slave_id INTEGER NOT NULL,
    setting_key VARCHAR(64) NOT NULL,
    setting_value TEXT,
    UNIQUE(slave_id, setting_key)
);

-- 迁移现有xrayTemplateConfig
INSERT INTO slave_settings (slave_id, setting_key, setting_value)
SELECT id, 'xrayTemplateConfig', (SELECT value FROM settings WHERE key='xrayTemplateConfig')
FROM slaves;
```

**后端修改:**
```go
// web/controller/xray_setting.go
func (a *XraySettingController) getXraySetting(c *gin.Context) {
    slaveIdStr := c.DefaultQuery("slaveId", "0")
    slaveId, _ := strconv.Atoi(slaveIdStr)
    
    if slaveId == 0 {
        jsonMsg(c, "请选择一个Slave", errors.New("slaveId required"))
        return
    }
    
    xraySetting, err := a.SettingService.GetXrayConfigTemplateForSlave(slaveId)
    // ... rest of code
}

func (a *XraySettingController) updateSetting(c *gin.Context) {
    slaveIdStr := c.PostForm("slaveId")
    slaveId, _ := strconv.Atoi(slaveIdStr)
    
    if slaveId == 0 {
        jsonMsg(c, "请选择一个Slave", errors.New("slaveId required"))
        return
    }
    
    xraySetting := c.PostForm("xraySetting")
    err := a.XraySettingService.SaveXraySettingForSlave(slaveId, xraySetting)
    // ... rest of code
}
```

**前端修改:**
```javascript
// web/html/xray.html
async getXraySetting() {
  if (!this.selectedSlaveId) {
    this.$message.error('请先选择一个Slave');
    return;
  }
  
  const msg = await HttpUtil.post("/panel/xray/", 
    { slaveId: this.selectedSlaveId });  // ✅ 添加slaveId参数
  // ... rest of code
}

async updateXraySetting() {
  if (!this.selectedSlaveId) {
    this.$message.error('请先选择一个Slave');
    return;
  }
  
  const msg = await HttpUtil.post("/panel/xray/update", { 
    xraySetting: this.xraySetting,
    slaveId: this.selectedSlaveId  // ✅ 添加slaveId参数
  });
  // ... rest of code
}
```

#### 方案B: 禁用Master的Xray设置页面 (临时方案)

如果短期内无法实现方案A，建议：

1. 在xray设置页面强制要求选择slave
2. 如果未选择slave（即访问/panel/xray没有slaveId参数），显示错误提示
3. 禁用保存按钮并显示警告

**前端修改 (临时):**
```javascript
// web/html/xray.html mounted()
async mounted() {
  const urlParams = new URLSearchParams(window.location.search);
  const slaveIdParam = urlParams.get('slaveId');
  
  if (!slaveIdParam) {
    this.$message.error('该页面必须指定slaveId参数访问');
    this.saveBtnDisable = true;
    return;
  }
  
  // ... rest of code
}
```

---

### 优先级2 (中) - Outbound流量统计隔离

**后端修改:**
```go
// web/controller/xray_setting.go
func (a *XraySettingController) getOutboundsTraffic(c *gin.Context) {
    slaveIdStr := c.DefaultQuery("slaveId", "-1")
    slaveId, _ := strconv.Atoi(slaveIdStr)
    
    var outboundsTraffic interface{}
    var err error
    
    if slaveId == -1 {
        outboundsTraffic, err = a.OutboundService.GetAllOutboundsTraffic()
    } else {
        outboundsTraffic, err = a.OutboundService.GetOutboundsTrafficForSlave(slaveId)
    }
    
    if err != nil {
        jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getOutboundTrafficError"), err)
        return
    }
    jsonObj(c, outboundsTraffic, nil)
}
```

**前端修改:**
```javascript
// web/html/xray.html
async getOutboundsTraffic() {
  let url = "/panel/xray/getOutboundsTraffic";
  if (this.selectedSlaveId) {
    url += `?slaveId=${this.selectedSlaveId}`;
  }
  const msg = await HttpUtil.get(url);
  if (msg.success) {
    this.outboundsTraffic = msg.obj;
  }
}
```

---

### 优先级3 (中) - Reset Outbound流量隔离

**后端修改:**
```go
// web/controller/xray_setting.go
func (a *XraySettingController) resetOutboundsTraffic(c *gin.Context) {
    tag := c.PostForm("tag")
    slaveIdStr := c.PostForm("slaveId")
    slaveId, _ := strconv.Atoi(slaveIdStr)
    
    if slaveId == 0 {
        jsonMsg(c, "请指定slaveId", errors.New("slaveId required"))
        return
    }
    
    err := a.OutboundService.ResetOutboundTrafficForSlave(slaveId, tag)
    if err != nil {
        jsonMsg(c, I18nWeb(c, "pages.settings.toasts.resetOutboundTrafficError"), err)
        return
    }
    jsonObj(c, "", nil)
}
```

**前端修改:**
```javascript
// web/html/xray.html
async resetOutboundTraffic(index) {
  if (!this.selectedSlaveId) {
    this.$message.error('请先选择一个Slave');
    return;
  }
  
  let tag = "-alltags-";
  if (index >= 0) {
    tag = this.outboundData[index].tag ? this.outboundData[index].tag : ""
  }
  
  const msg = await HttpUtil.post("/panel/xray/resetOutboundsTraffic", { 
    tag: tag,
    slaveId: this.selectedSlaveId  // ✅ 添加slaveId参数
  });
  
  if (msg.success) {
    await this.refreshOutboundTraffic();
  }
}
```

---

## 📊 风险总结

| 问题 | 风险等级 | 影响范围 | 修复难度 |
|------|---------|---------|---------|
| Xray模板配置全局共享 | 🔴 高 | 所有slave | 高 (需要数据库结构变更) |
| Outbound流量统计未隔离 | 🟡 中 | 显示错误 | 低 (仅需添加参数) |
| Reset Outbound流量未隔离 | 🟡 中 | 可能误操作 | 低 (仅需添加参数) |

## ✅ 已正确隔离的功能

- ✅ Inbound列表查询和管理
- ✅ Outbound列表查询和管理
- ✅ Routing Rule列表查询和管理
- ✅ Xray服务重启控制

---

## 🎯 下一步行动建议

1. **立即修复 (优先级1)**
   - 添加前端校验：强制要求选择slave才能访问xray设置页面
   - 在保存前检查slaveId参数
   - 显示明确的警告信息

2. **短期修复 (1-2周)**
   - 实现outbound流量统计的slave过滤
   - 实现reset outbound流量的slave隔离

3. **长期优化 (1个月)**
   - 设计并实现per-slave的配置模板存储方案
   - 数据库迁移脚本
   - 全面测试各slave的配置隔离

---

## 测试建议

修复后需要进行以下测试：

1. **隔离性测试**
   - 修改Slave A的xray配置，验证Slave B不受影响
   - 查看Slave A的流量统计，验证只显示A的数据
   - 重置Slave A的outbound流量，验证Slave B不受影响

2. **边界测试**
   - 不选择slave时尝试访问xray设置页面
   - 传递无效的slaveId参数
   - 删除slave后访问其配置

3. **并发测试**
   - 同时修改多个slave的xray配置
   - 验证配置不会相互覆盖

---

**报告生成时间:** 2026-02-06  
**分析工程师:** AI Assistant  
**审核状态:** 待人工审核
