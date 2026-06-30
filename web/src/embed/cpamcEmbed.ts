const CPAMC_EMBED_QUERY_VALUE = 'cpamc';
const CPAMC_READY_MESSAGE = 'cpa-usage-keeper:ready';

const currentSearch = () => (typeof window === 'undefined' ? '' : window.location.search);

export const isCPAMCEmbed = (search = currentSearch()): boolean => {
  const params = new URLSearchParams(search);
  return params.get('embed') === CPAMC_EMBED_QUERY_VALUE;
};

export const cpamcEmbedSearch = (search = currentSearch()): '' | '?embed=cpamc' => (
  isCPAMCEmbed(search) ? '?embed=cpamc' : ''
);

export const notifyCPAMCEmbedReady = (search = currentSearch()): void => {
  if (!isCPAMCEmbed(search) || typeof window === 'undefined' || window.parent === window) return;
  window.parent.postMessage({ type: CPAMC_READY_MESSAGE }, '*');
};
