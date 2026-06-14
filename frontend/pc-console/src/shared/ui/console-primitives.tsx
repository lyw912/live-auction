import React, { useMemo, useRef, useState } from 'react';

type ButtonTone = 'default' | 'primary' | 'danger';
type ButtonSize = 'mini' | 'small' | 'default';

type ButtonProps = Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'type'> & {
  type?: 'primary' | 'secondary' | 'default';
  htmlType?: React.ButtonHTMLAttributes<HTMLButtonElement>['type'];
  status?: 'danger' | 'warning' | 'success';
  size?: ButtonSize;
  icon?: React.ReactNode;
  loading?: boolean;
};

function cx(...values: Array<string | false | null | undefined>) {
  return values.filter(Boolean).join(' ');
}

export function Button({
  children,
  className,
  disabled,
  htmlType = 'button',
  icon,
  loading,
  size = 'default',
  status,
  type,
  ...props
}: ButtonProps) {
  const tone: ButtonTone = status === 'danger' ? 'danger' : type === 'primary' ? 'primary' : 'default';
  return (
    <button
      {...props}
      aria-busy={loading || undefined}
      className={cx('console-button', `console-button-${tone}`, `console-button-${size}`, className)}
      disabled={disabled || loading}
      type={htmlType}
    >
      {icon ? <span className="console-button-icon">{icon}</span> : null}
      <span className="console-button-label">{children}</span>
    </button>
  );
}

function LayoutRoot({ children, className }: { children: React.ReactNode; className?: string }) {
  return <section className={className}>{children}</section>;
}

function LayoutSider({
  children,
  className,
  width
}: {
  children: React.ReactNode;
  className?: string;
  width?: number;
}) {
  return <aside className={className} style={width ? { width, flex: `0 0 ${width}px` } : undefined}>{children}</aside>;
}

function LayoutContent({ children, className }: { children: React.ReactNode; className?: string }) {
  return <main className={className}>{children}</main>;
}

export const Layout = Object.assign(LayoutRoot, {
  Sider: LayoutSider,
  Content: LayoutContent
});

type MessageKind = 'error' | 'success' | 'info' | 'warning';

function ensureMessageStack() {
  let stack = document.querySelector<HTMLDivElement>('.console-message-stack');
  if (!stack) {
    stack = document.createElement('div');
    stack.className = 'console-message-stack';
    stack.setAttribute('role', 'status');
    stack.setAttribute('aria-live', 'polite');
    document.body.appendChild(stack);
  }
  return stack;
}

function showMessage(kind: MessageKind, content: React.ReactNode) {
  if (typeof document === 'undefined') return;
  const item = document.createElement('div');
  item.className = `console-message console-message-${kind}`;
  item.textContent = typeof content === 'string' || typeof content === 'number' ? String(content) : '';
  ensureMessageStack().appendChild(item);
  window.setTimeout(() => item.remove(), 3600);
}

export const Message = {
  error: (content: React.ReactNode) => showMessage('error', content),
  success: (content: React.ReactNode) => showMessage('success', content),
  info: (content: React.ReactNode) => showMessage('info', content),
  warning: (content: React.ReactNode) => showMessage('warning', content)
};

export const Modal = {
  confirm({
    cancelText = '取消',
    content,
    okText = '确定',
    okButtonProps,
    onOk,
    title
  }: {
    cancelText?: string;
    content?: React.ReactNode;
    okText?: string;
    okButtonProps?: { status?: 'danger' | 'warning' | 'success' };
    onOk?: () => void | Promise<void>;
    title: React.ReactNode;
  }) {
    if (typeof document === 'undefined') return;
    const overlay = document.createElement('div');
    overlay.className = 'console-modal-backdrop';
    const dialog = document.createElement('section');
    dialog.className = 'console-modal';
    dialog.setAttribute('role', 'dialog');
    dialog.setAttribute('aria-modal', 'true');
    dialog.setAttribute('aria-label', typeof title === 'string' ? title : '确认');
    const heading = document.createElement('h2');
    heading.textContent = typeof title === 'string' ? title : '确认';
    const body = document.createElement('p');
    body.textContent = typeof content === 'string' || typeof content === 'number' ? String(content) : '';
    const actions = document.createElement('div');
    actions.className = 'console-modal-actions';
    const cancel = document.createElement('button');
    cancel.className = 'console-button console-button-default console-button-default';
    cancel.type = 'button';
    cancel.textContent = cancelText;
    const ok = document.createElement('button');
    ok.className = 'console-button console-button-primary console-button-default';
    if (okButtonProps?.status === 'danger') {
      ok.className = 'console-button console-button-danger console-button-default';
    }
    ok.type = 'button';
    ok.textContent = okText;
    const close = () => overlay.remove();
    cancel.addEventListener('click', close);
    ok.addEventListener('click', () => {
      Promise.resolve(onOk?.()).finally(close);
    });
    actions.append(cancel, ok);
    dialog.append(heading, body, actions);
    overlay.appendChild(dialog);
    document.body.appendChild(overlay);
  }
};

type DrawerProps = {
  children: React.ReactNode;
  className?: string;
  footer?: React.ReactNode;
  onCancel?: () => void;
  title?: React.ReactNode;
  unmountOnExit?: boolean;
  visible?: boolean;
  width?: number;
};

export function Drawer({ children, className, footer, onCancel, title, visible, width = 520 }: DrawerProps) {
  if (!visible) return null;
  return (
    <div className="console-drawer-backdrop">
      <aside
        aria-label={typeof title === 'string' ? title : '抽屉'}
        aria-modal="true"
        className={cx('console-drawer', className)}
        role="dialog"
        style={{ width }}
      >
        <header className="console-drawer-head">
          <h2>{title}</h2>
          <button aria-label="关闭" className="console-drawer-close" type="button" onClick={onCancel}>×</button>
        </header>
        <div className="console-drawer-body">{children}</div>
        {footer ? <footer className="console-drawer-foot">{footer}</footer> : null}
      </aside>
    </div>
  );
}

type FormRootProps = React.FormHTMLAttributes<HTMLFormElement> & {
  layout?: 'vertical' | 'horizontal';
};

function FormRoot({ children, className, layout = 'vertical', ...props }: FormRootProps) {
  return <form {...props} className={cx('console-form', `console-form-${layout}`, className)}>{children}</form>;
}

function FormItem({
  children,
  help,
  label,
  validateStatus
}: {
  children: React.ReactNode;
  help?: React.ReactNode;
  label?: React.ReactNode;
  validateStatus?: 'success' | 'warning' | 'error';
}) {
  return (
    <label className={cx('console-form-item', validateStatus ? `console-form-item-${validateStatus}` : '')}>
      {label ? <span className="console-form-label">{label}</span> : null}
      {children}
      {help ? <span className="console-form-help">{help}</span> : null}
    </label>
  );
}

export const Form = Object.assign(FormRoot, { Item: FormItem });

type InputProps = Omit<React.InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'prefix'> & {
  onChange?: (value: string) => void;
  prefix?: React.ReactNode;
};

function InputRoot({ className, onChange, prefix, ...props }: InputProps) {
  const input = (
    <input
      {...props}
      className={cx('console-input', prefix ? 'console-input-with-prefix' : '', className)}
      onChange={(event) => onChange?.(event.currentTarget.value)}
    />
  );
  if (!prefix) return input;
  return (
    <span className="console-input-shell">
      <span className="console-input-prefix">{prefix}</span>
      {input}
    </span>
  );
}

function TextArea({
  autoSize,
  className,
  onChange,
  ...props
}: Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
  autoSize?: { minRows?: number; maxRows?: number } | boolean;
  onChange?: (value: string) => void;
}) {
  const rows = typeof autoSize === 'object' ? autoSize.minRows : props.rows;
  return (
    <textarea
      {...props}
      className={cx('console-input console-textarea', className)}
      rows={rows}
      onChange={(event) => onChange?.(event.currentTarget.value)}
    />
  );
}

export const Input = Object.assign(InputRoot, { TextArea });

type InputNumberProps = Omit<React.InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'prefix' | 'value'> & {
  onChange?: (value: number | null) => void;
  precision?: number;
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
  value?: number | null;
};

export function InputNumber({ className, onChange, precision, prefix, suffix, value, ...props }: InputNumberProps) {
  const step = props.step ?? (precision != null ? 1 / (10 ** precision) : 1);
  return (
    <span className="console-number-shell">
      {prefix ? <span className="console-number-affix">{prefix}</span> : null}
      <input
        {...props}
        className={cx('console-input console-number-input', className)}
        step={step}
        type="number"
        value={value ?? ''}
        onChange={(event) => {
          const next = event.currentTarget.value;
          onChange?.(next === '' ? null : Number(next));
        }}
      />
      {suffix ? <span className="console-number-affix">{suffix}</span> : null}
    </span>
  );
}

type DatePickerProps = Omit<React.InputHTMLAttributes<HTMLInputElement>, 'onChange'> & {
  format?: string;
  onChange?: (value: string) => void;
  showTime?: boolean;
  value?: string;
};

export function DatePicker({ className, onChange, placeholder = '请选择日期', value, ...props }: DatePickerProps) {
  return (
    <input
      {...props}
      className={cx('console-input', className)}
      placeholder={placeholder}
      type="text"
      value={value ?? ''}
      onChange={(event) => onChange?.(event.currentTarget.value)}
    />
  );
}

export function Space({
  children,
  className,
  wrap
}: {
  children: React.ReactNode;
  className?: string;
  wrap?: boolean;
}) {
  return <div className={cx('console-space', wrap ? 'console-space-wrap' : '', className)}>{children}</div>;
}

export function Tag({ children, className, color }: { children: React.ReactNode; className?: string; color?: string }) {
  return <span className={cx('console-tag', color ? `console-tag-${color}` : '', className)} data-color={color}>{children}</span>;
}

type UploadRequestOption = {
  file: File;
  onSuccess?: (response: unknown) => void;
};

export function Upload({
  accept,
  children,
  customRequest,
  showUploadList = true
}: {
  accept?: string;
  children: React.ReactElement;
  customRequest?: (option: UploadRequestOption) => { abort?: () => void } | void;
  limit?: number;
  showUploadList?: boolean;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [fileName, setFileName] = useState('');
  const trigger = (event: React.MouseEvent) => {
    event.preventDefault();
    inputRef.current?.click();
  };
  const child = React.cloneElement(children, {
    onClick: trigger
  } as Partial<React.HTMLAttributes<HTMLElement>>);
  return (
    <span className="console-upload">
      <input
        ref={inputRef}
        accept={accept}
        className="console-upload-input"
        type="file"
        onChange={(event) => {
          const file = event.currentTarget.files?.[0];
          if (!file) return;
          setFileName(file.name);
          customRequest?.({ file, onSuccess: () => undefined });
          event.currentTarget.value = '';
        }}
      />
      {child}
      {showUploadList && fileName ? <span className="console-upload-file">{fileName}</span> : null}
    </span>
  );
}

type TableColumn<T extends Record<string, unknown>> = {
  dataIndex?: string;
  render?: (value: unknown, record: T) => React.ReactNode;
  title: React.ReactNode;
};

export function Table<T extends Record<string, unknown>>({
  columns,
  data,
  rowKey
}: {
  columns: Array<TableColumn<T>>;
  data: T[];
  pagination?: false;
  rowKey?: ((record: T) => string) | string;
}) {
  const rows = data ?? [];
  return (
    <div className="console-table-wrap">
      <table className="console-table">
        <thead>
          <tr>{columns.map((column, index) => <th key={index}>{column.title}</th>)}</tr>
        </thead>
        <tbody>
          {rows.map((record, rowIndex) => {
            const key = typeof rowKey === 'function'
              ? rowKey(record)
              : rowKey
                ? String(record[rowKey])
                : String(rowIndex);
            return (
              <tr key={key}>
                {columns.map((column, columnIndex) => {
                  const value = column.dataIndex ? record[column.dataIndex] : undefined;
                  return <td key={columnIndex}>{column.render ? column.render(value, record) : String(value ?? '')}</td>;
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

type TabPaneProps = {
  children: React.ReactNode;
  title: React.ReactNode;
};

function TabPane({ children }: TabPaneProps) {
  return <>{children}</>;
}

function TabsRoot({ children, defaultActiveTab }: { children: React.ReactNode; defaultActiveTab?: string }) {
  const panes = useMemo(
    () => React.Children.toArray(children).filter(React.isValidElement) as Array<React.ReactElement<TabPaneProps>>,
    [children]
  );
  const firstKey = panes[0]?.key != null ? String(panes[0].key) : '';
  const [active, setActive] = useState(defaultActiveTab ?? firstKey);
  const activePane = panes.find((pane) => String(pane.key) === active) ?? panes[0];
  return (
    <div className="console-tabs">
      <div className="console-tab-list" role="tablist">
        {panes.map((pane) => {
          const key = String(pane.key);
          const selected = key === String(activePane?.key);
          return (
            <button
              aria-selected={selected}
              className={cx('console-tab', selected ? 'console-tab-active' : '')}
              key={key}
              role="tab"
              type="button"
              onClick={() => setActive(key)}
            >
              {pane.props.title}
            </button>
          );
        })}
      </div>
      {activePane ? (
        <section aria-label={typeof activePane.props.title === 'string' ? activePane.props.title : undefined} className="console-tab-panel" role="tabpanel">
          {activePane.props.children}
        </section>
      ) : null}
    </div>
  );
}

export const Tabs = Object.assign(TabsRoot, { TabPane });
