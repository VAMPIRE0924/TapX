import type { CSSProperties, KeyboardEvent, ReactNode } from 'react';
import './InputAddon.css';

interface InputAddonProps {
  children: ReactNode;
  className?: string;
  style?: CSSProperties;
  onClick?: () => void;
  ariaLabel?: string;
}

export function InputAddon({ children, className = '', style, onClick, ariaLabel }: InputAddonProps) {
  function onKeyDown(event: KeyboardEvent<HTMLSpanElement>) {
    if (!onClick) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onClick();
    }
  }

  return (
    <span
      className={`input-addon ${className}`.trim()}
      style={style}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      aria-label={onClick ? ariaLabel : undefined}
      onKeyDown={onKeyDown}
    >
      {children}
    </span>
  );
}
