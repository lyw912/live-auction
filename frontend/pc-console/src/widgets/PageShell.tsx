import React from 'react';

export function PageShell({
  title,
  description,
  toolbar,
  children
}: {
  title: string;
  description?: string;
  toolbar?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="pc-page-shell">
      <header className="pc-page-shell-head">
        <div>
          <h1>{title}</h1>
          {description ? <p>{description}</p> : null}
        </div>
        {toolbar ? <div className="pc-page-shell-toolbar">{toolbar}</div> : null}
      </header>
      {children}
    </section>
  );
}
