import { useEffect, useState } from 'react';
import { fetchStatus } from '@/lib/api';
import { IconGithub } from '@/components/ui/icons';

export function footerVersionLabel(version?: string): string | undefined {
  const trimmed = version?.trim();
  return trimmed ? `Version: ${trimmed}` : undefined;
}

export function AppFooter({ version: fixedVersion }: { version?: string }) {
  const [version, setVersion] = useState('');

  useEffect(() => {
    if (fixedVersion !== undefined) return;

    let cancelled = false;
    void fetchStatus()
      .then((status) => {
        if (!cancelled) setVersion(status.version ?? '');
      })
      .catch(() => {
        if (!cancelled) setVersion('');
      });

    return () => {
      cancelled = true;
    };
  }, [fixedVersion]);

  const versionLabel = footerVersionLabel(fixedVersion ?? version);

  return (
    <footer className="app-footer">
      <div className="app-footer-line app-footer-meta">
        <span>© 2026</span>
        <a href="https://github.com/Willxup/cpa-usage-keeper" target="_blank" rel="noreferrer">CPA Usage Keeper</a>
        <span>·</span>
        <a href="https://github.com/Willxup/cpa-usage-keeper/blob/main/LICENSE" target="_blank" rel="noreferrer">License</a>
        <span>·</span>
        <a href="https://github.com/router-for-me/CLIProxyAPI" target="_blank" rel="noreferrer">CLIProxyAPI Integration</a>
      </div>
      <div className="app-footer-line app-footer-powered">
        <span>Powered By</span>
        <a href="https://github.com/Willxup" target="_blank" rel="noreferrer" aria-label="Willxup GitHub profile">
          <IconGithub size={16} aria-hidden="true" />
          <span>Willxup</span>
        </a>
        {versionLabel ? (
          <>
            <span>·</span>
            <span className="app-footer-version">{versionLabel}</span>
          </>
        ) : null}
      </div>
    </footer>
  );
}
