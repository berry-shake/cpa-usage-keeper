import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError, deletePricing, fetchPricing, fetchUsedModels, syncRemotePricing, updatePricing } from '@/lib/api';
import { useNotificationStore } from '@/stores';
import { loadModelPrices, saveModelPrices, type ModelPrice } from '@/utils/usage';

export interface PricingSyncMeta {
  sourceUrl: string;
  sourceUrls: string[];
  importedCount: number;
  matchedCount: number;
  updatedCount: number;
  unmatchedModels: string[];
  syncedAt: string;
}

export interface UsePricingDataOptions {
  onAuthRequired?: () => void;
  enabled?: boolean;
}

export interface UsePricingDataReturn {
  modelNames: string[];
  modelPrices: Record<string, ModelPrice>;
  loading: boolean;
  error: string;
  syncingPrices: boolean;
  syncMeta: PricingSyncMeta | null;
  lastRefreshedAt: Date | null;
  loadPricing: () => Promise<void>;
  setModelPrices: (prices: Record<string, ModelPrice>) => Promise<void>;
  syncRemoteModelPrices: () => Promise<void>;
}

const pricingToModelPrice = (entry: {
  model: string;
  prompt_price_per_1m: number;
  completion_price_per_1m: number;
  cache_price_per_1m: number;
}): ModelPrice => ({
  prompt: entry.prompt_price_per_1m,
  completion: entry.completion_price_per_1m,
  cache: entry.cache_price_per_1m,
});

export function usePricingData(options: UsePricingDataOptions = {}): UsePricingDataReturn {
  const { onAuthRequired, enabled = true } = options;
  const { t } = useTranslation();
  const { showNotification } = useNotificationStore();
  const [modelNames, setModelNames] = useState<string[]>([]);
  const [modelPrices, setModelPricesState] = useState<Record<string, ModelPrice>>(() => loadModelPrices());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [syncingPrices, setSyncingPrices] = useState(false);
  const [syncMeta, setSyncMeta] = useState<PricingSyncMeta | null>(null);
  const [lastRefreshedAt, setLastRefreshedAt] = useState<Date | null>(null);
  const requestControllerRef = useRef<AbortController | null>(null);

  const loadPricing = useCallback(async () => {
    requestControllerRef.current?.abort();
    const controller = new AbortController();
    requestControllerRef.current = controller;

    setLoading(true);
    setError('');

    try {
      const [pricingResponse, usedModelsResponse] = await Promise.all([
        fetchPricing(controller.signal),
        fetchUsedModels(controller.signal),
      ]);
      if (requestControllerRef.current !== controller) {
        return;
      }
      const prices = Object.fromEntries(
        pricingResponse.pricing.map((entry) => [entry.model, pricingToModelPrice(entry)])
      );
      saveModelPrices(prices);
      setModelPricesState(prices);
      setModelNames(usedModelsResponse.models);
      setLastRefreshedAt(new Date());
    } catch (error) {
      if (controller.signal.aborted) {
        return;
      }
      if (error instanceof ApiError && error.status === 401) {
        onAuthRequired?.();
        return;
      }
      setModelPricesState(loadModelPrices());
      setError(error instanceof Error ? error.message : 'Failed to load pricing');
    } finally {
      if (requestControllerRef.current === controller) {
        setLoading(false);
        requestControllerRef.current = null;
      }
    }
  }, [onAuthRequired]);

  useEffect(() => {
    if (!enabled) {
      requestControllerRef.current?.abort();
      requestControllerRef.current = null;
      setLoading(false);
      return;
    }
    void loadPricing();
    return () => {
      requestControllerRef.current?.abort();
      requestControllerRef.current = null;
    };
  }, [enabled, loadPricing]);

  const setModelPrices = useCallback(async (prices: Record<string, ModelPrice>) => {
    const previousPrices = modelPrices;
    setModelPricesState(prices);
    saveModelPrices(prices);

    try {
      const previousModels = new Set(Object.keys(previousPrices));
      const nextModels = new Set(Object.keys(prices));
      await Promise.all([
        ...Object.entries(prices).map(([model, pricing]) =>
          updatePricing(model, {
            prompt_price_per_1m: pricing.prompt,
            completion_price_per_1m: pricing.completion,
            cache_price_per_1m: pricing.cache,
          })
        ),
        ...Array.from(previousModels)
          .filter((model) => !nextModels.has(model))
          .map((model) => deletePricing(model)),
      ]);
      setLastRefreshedAt(new Date());
    } catch (error) {
      setModelPricesState(previousPrices);
      saveModelPrices(previousPrices);
      if (error instanceof ApiError && error.status === 401) {
        onAuthRequired?.();
        return;
      }
      const message = error instanceof Error ? error.message : '';
      showNotification(
        `${t('notification.upload_failed')}${message ? `: ${message}` : ''}`,
        'error'
      );
    }
  }, [modelPrices, onAuthRequired, showNotification, t]);

  const syncRemoteModelPrices = useCallback(async () => {
    if (syncingPrices) {
      return;
    }

    setSyncingPrices(true);
    setError('');

    try {
      const response = await syncRemotePricing();
      const syncedPrices = Object.fromEntries(
        response.pricing.map((entry) => [entry.model, pricingToModelPrice(entry)])
      );
      const nextPrices = {
        ...modelPrices,
        ...syncedPrices,
      };
      setModelPricesState(nextPrices);
      saveModelPrices(nextPrices);
      setSyncMeta({
        sourceUrl: response.source_url,
        sourceUrls: response.source_urls,
        importedCount: response.imported_count,
        matchedCount: response.matched_count,
        updatedCount: response.updated_count,
        unmatchedModels: response.unmatched_models,
        syncedAt: response.synced_at,
      });
      setLastRefreshedAt(new Date());

      if (response.matched_count > 0) {
        showNotification(t('usage_stats.model_price_sync_success', { count: response.matched_count }));
      } else {
        showNotification(t('usage_stats.model_price_sync_empty'), 'error');
      }
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        onAuthRequired?.();
        return;
      }
      const message = error instanceof Error ? error.message : '';
      showNotification(
        `${t('usage_stats.model_price_sync_failed')}${message ? `: ${message}` : ''}`,
        'error'
      );
      setError(error instanceof Error ? error.message : 'Failed to sync pricing');
    } finally {
      setSyncingPrices(false);
    }
  }, [modelPrices, onAuthRequired, showNotification, syncingPrices, t]);

  return {
    modelNames,
    modelPrices,
    loading,
    error,
    syncingPrices,
    syncMeta,
    lastRefreshedAt,
    loadPricing,
    setModelPrices,
    syncRemoteModelPrices,
  };
}
