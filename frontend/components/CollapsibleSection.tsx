import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";

type CollapsibleSectionProps = {
  title: string;
  summary: string;
  open: boolean;
  onToggle: () => void;
  headerControls?: ReactNode;
  children: ReactNode;
};

export function CollapsibleSection({ title, summary, open, onToggle, headerControls, children }: CollapsibleSectionProps) {
  return (
    <section className="collapsible-section section" data-section-title={title}>
      <div className="collapsible-header">
        <div className="collapsible-heading">
          <h2>{title}</h2>
          <p>{summary}</p>
        </div>
        <div className="collapsible-actions">
          {headerControls}
          <Button type="button" variant="outline" className="collapsible-toggle" aria-expanded={open} onClick={onToggle}>
            <span className="collapsible-toggle-icon" aria-hidden="true">
              {open ? "-" : "+"}
            </span>
            {open ? "閉じる" : "開く"}
          </Button>
        </div>
      </div>
      {open ? <div className="collapsible-body">{children}</div> : null}
    </section>
  );
}
