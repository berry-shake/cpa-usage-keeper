import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { MainActionButton } from '../MainActionButton';

describe('MainActionButton', () => {
  it('forwards button state and custom classes', () => {
    const html = renderToStaticMarkup(
      <MainActionButton
        shellClassName="page-action-shell"
        className="page-action-trigger"
        loading
        data-page-action="refresh"
      >
        Refresh
      </MainActionButton>,
    );

    expect(html).toContain('class="main-action-button-shell page-action-shell"');
    expect(html).toContain('class="btn btn-primary btn-action main-action-button page-action-trigger"');
    expect(html).toContain('data-page-action="refresh"');
    expect(html).toContain('aria-busy="true"');
    expect(html).toContain('disabled=""');
    expect(html).toContain('class="loading-spinner"');
  });

});
