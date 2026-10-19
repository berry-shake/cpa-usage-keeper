import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { CLIPROXYAPI_REPOSITORY_URL, GITHUB_PROFILE_URL, GITHUB_REPOSITORY_URL } from '@/utils/constants';
import { AppFooter, footerVersionLabel, loadFooterVersion } from '../AppFooter';

describe('AppFooter', () => {
  it('renders project links, powered by line, and version label', () => {
    const html = renderToStaticMarkup(<AppFooter version="v1.2.3" />);

    expect(html).toContain(`href="${GITHUB_REPOSITORY_URL}"`);
    expect(html).toContain('>CPA Usage Keeper</a>');
    expect(html).toContain('License');
    expect(html).toContain('CLIProxyAPI Integration');
    expect(html).toContain('Powered By');
    expect(html).toContain('aria-label="Willxup GitHub profile"');
    expect(html).toContain('Willxup');
    expect(html).toContain('Version: v1.2.3');
    expect(html).toContain(`href="${GITHUB_REPOSITORY_URL}/blob/main/LICENSE"`);
    expect(html).toContain(`href="${CLIPROXYAPI_REPOSITORY_URL}"`);
    expect(html).toContain(`href="${GITHUB_PROFILE_URL}"`);
  });

  it('does not render a version label before the version is available', () => {
    const html = renderToStaticMarkup(<AppFooter />);

    expect(html).not.toContain('Version:');
  });

  it('respects disabled version loading while still allowing a fixed version', () => {
    const unloadedHtml = renderToStaticMarkup(<AppFooter loadVersion={false} />);
    const fixedHtml = renderToStaticMarkup(<AppFooter version="v1.2.3" loadVersion={false} />);

    expect(unloadedHtml).not.toContain('Version:');
    expect(fixedHtml).toContain('Version: v1.2.3');
  });

  it('formats only non-empty version values', () => {
    expect(footerVersionLabel('v1.2.3')).toBe('Version: v1.2.3');
    expect(footerVersionLabel('dev')).toBe('Version: dev');
    expect(footerVersionLabel('')).toBeUndefined();
    expect(footerVersionLabel(undefined)).toBeUndefined();
  });

  it('loads footer version through the provided version loader', async () => {
    const signal = new AbortController().signal;
    const loadVersion = vi.fn(async () => ({ version: 'v1.2.3', updateCheckEnabled: true }));

    await expect(loadFooterVersion(loadVersion, signal)).resolves.toBe('v1.2.3');

    expect(loadVersion).toHaveBeenCalledWith(signal);
  });

  it('falls back to an empty footer version when version loading fails', async () => {
    const signal = new AbortController().signal;
    const loadVersion = vi.fn().mockRejectedValue(new Error('network failed'));

    await expect(loadFooterVersion(loadVersion, signal)).resolves.toBe('');
  });

});
