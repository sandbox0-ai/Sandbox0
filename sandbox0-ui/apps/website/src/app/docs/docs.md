# 文档语法与组件说明（Markdown / MDX）

本目录（`apps/website/src/app/docs/`）下的页面以 **MDX** 为主：在 Markdown 基础语法上允许直接写 JSX 组件（例如 `<Callout />`、`<Tabs />`）。本文档列出 **当前站点已接入的全部语法与组件**，并说明各自的使用场景与视觉效果。渲染逻辑以 `apps/website/src/components/docs/MDXComponents.tsx` 为准。

---

## 1. 文档页面与组织方式

- **页面入口**：每个路由页面必须使用 `page.mdx`（例如 `apps/website/src/app/docs/<slug>/page.mdx`）。
- **非路由内容**：`*.md` 或 `*.mdx` 可用于存档或协作，但不会自动成为路由页面。

---

## 2. Markdown 基础语法（自动套像素风样式）

### 2.1 标题（Headings）

用于页面结构分层，站点会自动套用像素风标题组件。

    # 一级标题
    ## 二级标题
    ### 三级标题
    #### 四级标题

效果：`h1` 会带特殊前缀标记，`h2`~`h4` 统一为像素风标题样式。

### 2.2 段落（Paragraph）

用于正文叙述。普通换行分段即可：

    这是第一段。
    
    这是第二段。

效果：段落默认行距更舒展，文本为次强调色（便于阅读）。

### 2.3 列表（Lists）

用于步骤、要点或约束说明。

无序列表：

    - 条目 1
    - 条目 2
      - 子条目

有序列表：

    1. 第一步
    2. 第二步

效果：无序列表自动显示像素风小方块前缀；有序列表为标准数字序列。

### 2.4 链接（Links）

用于跳转到外部站点或文档内页面。

    [Sandbox0 官网](https://sandbox0.ai)

效果：链接使用强调色并带 hover 过渡。

### 2.5 行内代码（Inline code）

用于表达命令、参数、配置项或关键字。

    使用 `s0` CLI 创建沙箱。

效果：带轻量背景、像素风边框与等宽字体。

### 2.6 代码块（Fenced code block）

用于完整命令、示例代码或配置片段。推荐写语言标签以启用高亮。

    ```bash
    s0 create --template python
    ```

效果：自动渲染为像素风代码块，支持复制按钮与语法高亮。

可识别的语言（含常见别名）：

- `bash`（别名：`sh` / `zsh` / `shell`）
- `json`
- `javascript`（别名：`js` / `jsx`）
- `typescript`（别名：`ts` / `tsx`）
- `python`
- `yaml`（别名：`yml`）

### 2.7 引用（Blockquote）

用于警示、背景信息或补充说明。

    > 这是一段引用说明。

效果：左侧彩色竖线与倾斜文本。

### 2.8 表格（Tables）

用于字段、参数或对比说明。

    | 字段 | 说明 |
    | ---- | ---- |
    | id   | 沙箱 ID |
    | name | 名称 |

效果：表格带边框与表头底色，适合参数说明。

### 2.9 分割线（Horizontal rule）

用于章节隔断或页面结构分隔。

    ---

效果：细边线分割，留足上下间距。

---

## 3. 自定义 MDX 组件（无需 import）

下列组件可直接在 `*.mdx` 中使用，适用于布局、强调、组件化展示。

### 3.1 `Callout`

使用场景：提示、注意、警告或结论卡片。

    <Callout type="info" title="提示">
      这里是说明内容。
    </Callout>

效果：像素风提示卡，自动带上下间距。

可用参数：

- `type`：`info` / `success` / `warning` / `danger`
- `title`：可选标题
- `className`：自定义样式（谨慎使用）

### 3.2 `Tabs`

使用场景：多语言代码示例、不同配置选项、不同平台说明。

    <Tabs
      tabs={[
        { label: "Python", content: <CodeBlock language="bash">{`pip install sandbox0`}</CodeBlock> },
        { label: "JavaScript", content: <CodeBlock language="bash">{`npm install @sandbox0/sdk`}</CodeBlock> },
      ]}
    />

效果：像素风选项卡，可在整个文档站点内同步用户选择（同名标签会联动）。

可用参数：

- `tabs`：必填，数组 `{ label, content }`
- `defaultTab`：可选，默认选中索引
- `className`：可选

### 3.3 `CodeBlock`

使用场景：需要在 JSX 结构中嵌套代码块（例如搭配 `Tabs`）。

    <CodeBlock language="python" filename="example.py" scale="md">
      {`print("hello")`}
    </CodeBlock>

效果：像素风代码块，带复制按钮、语言/文件名标签。

可用参数：

- `language`：语言名称（同 2.6）
- `filename`：显示在顶部的文件名
- `showLineNumbers`：是否显示行号
- `scale`：尺寸（继承 UI 组件）
- `accent`：是否使用强调阴影

### 3.4 `Badge`

使用场景：标记状态、版本、标签。

    <Badge variant="accent" size="md">Beta</Badge>

效果：像素风徽章。

常用参数（来自 UI 组件）：

- `variant`：`accent` / `success` / `warning` / `danger` / `default`
- `size`：`sm` / `md` / `lg`

### 3.5 `LinkRow`

使用场景：横向链接列表（官网、GitHub、Discord 等）。

    <LinkRow links="Discord=https://discord.gg/sandbox0|GitHub=https://github.com/sandbox0|Email=mailto:support@sandbox0.ai" />

效果：横向排列链接，自动间距与 hover 样式。

可用参数：

- `links`：`label=url` 形式，多个链接用 `|` 分隔
- `className`：外层容器样式
- `linkClassName`：单个链接样式

### 3.6 `ResourceList` / `ResourceItem`

使用场景：资源或 SDK 列表（徽章 + 描述 + 右侧 CTA）。

    <ResourceList>
      <ResourceItem
        badge="Python"
        description="Full-featured Python SDK with async support"
        href="/docs/sdks/python"
      />
      <ResourceItem
        badge="Go"
        description="High-performance Go SDK with full API coverage"
        href="/docs/sdks/go"
        cta="查看文档 →"
      />
    </ResourceList>

效果：统一的行式布局，便于列表对齐。

可用参数（`ResourceItem`）：

- `badge`：左侧徽章文本
- `description`：中部说明
- `href`：右侧链接
- `cta`：链接文案（默认 `Docs →`）

### 3.7 `TerminalBlock`

使用场景：CLI 输出或终端日志。

    <TerminalBlock lines={"$ s0 create --template python\n✓ Sandbox created in 98ms\nsandbox-id: sb_abc123"} />

效果：终端风格块，并根据行首符号自动着色。

可用参数：

- `lines`：使用 `\n` 分隔的多行字符串

自动着色规则：

- `$` 开头：强调色（命令）
- `✓` 或 `success` 开头：绿色（成功）
- `✕` 或 `error` 开头：红色（错误）
- 其他：弱化色（普通输出）

### 3.8 `Endpoint`

使用场景：API 路由展示（方法 + 路径）。

    <Endpoint method="GET">/sandboxes/:id</Endpoint>
    <Endpoint method="POST">/sandboxes</Endpoint>

效果：方法名像素徽章 + 路径等宽展示，并按方法类型着色。

可用参数：

- `method`：`GET` / `POST` / `PUT` / `PATCH` / `DELETE`
- `children`：路径文本

### 3.9 Landing 页面组件

使用场景：文档入口页或主题聚合页。

- `DocsHero`：页面头部大标题 + 说明
- `CardGrid`：卡片栅格布局
- `LinkCard`：可点击的入口卡片

示例：

    <DocsHero title="DOCUMENTATION">
      这里是一段简介。
    </DocsHero>
    
    <CardGrid>
      <LinkCard title="🚀 QUICK START" href="/docs/quickstart" cta="View Guide">
        Get your first sandbox running in under 5 minutes.
      </LinkCard>
    </CardGrid>

---

## 4. 推荐写法（减少 JSX 负担）

- **优先使用 Markdown**：能用列表、链接、fenced code 解决的就不要写 JSX。
- **需要结构再用 MDX**：例如 `Tabs`、`Callout`、`LinkRow`、`ResourceList`。
- **避免手写容器**：常见布局优先使用已有语法糖组件；如果缺少语法糖，先补组件再写文档。

