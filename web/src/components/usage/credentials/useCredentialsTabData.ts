import { useMemo } from 'react'
import {
  buildAiProviderCredentialRows,
  buildAuthFileCredentialRows,
  selectQuotaEligibleAuthIndexes,
  type AiProviderCredentialRow,
  type AuthFileCredentialRow,
} from './credentialViewModels'
import { useCredentialPages } from './useCredentialPages'
import { useQuotaCache } from './useQuotaCache'
import type { UsageIdentityPageSort } from '@/lib/api'
import type { UsageIdentityTypeCount } from '@/lib/types'
import { quotaRefreshDisplayError, useQuotaRefreshTasks } from './useQuotaRefreshTasks'
import type { CredentialProviderFilterKey } from './credentialProviderFilters'

interface UseCredentialsTabDataOptions {
  enabledAuthFiles: boolean
  enabledAiProviders: boolean
  onAuthRequired?: () => void
}

export interface CredentialsTabData {
  authFileRows: AuthFileCredentialRow[]
  aiProviderRows: AiProviderCredentialRow[]
  authFileTypeCounts: UsageIdentityTypeCount[]
  aiProviderTypeCounts: UsageIdentityTypeCount[]
  authFileTotal: number
  aiProviderTotal: number
  authFilePageSize: number
  aiProviderPageSize: number
  authFilePage: number
  aiProviderPage: number
  authFileTotalPages: number
  aiProviderTotalPages: number
  authFileActiveOnly: boolean
  authFileProviderFilter: CredentialProviderFilterKey
  aiProviderProviderFilter: CredentialProviderFilterKey
  authFileSort: UsageIdentityPageSort
  aiProviderSort: UsageIdentityPageSort
  setAuthFilePage: (page: number) => void
  setAiProviderPage: (page: number) => void
  setAuthFilePageSize: (pageSize: number) => void
  setAiProviderPageSize: (pageSize: number) => void
  setAuthFileActiveOnly: (activeOnly: boolean) => void
  setAuthFileProviderFilter: (filter: CredentialProviderFilterKey) => void
  setAiProviderProviderFilter: (filter: CredentialProviderFilterKey) => void
  setAuthFileSort: (sort: UsageIdentityPageSort) => void
  setAiProviderSort: (sort: UsageIdentityPageSort) => void
  loading: boolean
  error: string
  quotaRefreshing: boolean
  quotaRefreshError: string
  refresh: () => Promise<void>
  refreshQuotaForCurrentAuthFilePage: () => Promise<void>
  refreshQuotaForAuthIndex: (authIndex: string) => Promise<void>
}

export function useCredentialsTabData({ enabledAuthFiles, enabledAiProviders, onAuthRequired }: UseCredentialsTabDataOptions): CredentialsTabData {
  // 页面 hook 只编排分页、缓存和刷新任务三层数据，不直接发散 API 调用。
  const credentialPages = useCredentialPages({ enabledAuthFiles, enabledAiProviders, onAuthRequired })
  const currentAuthIndexes = useMemo(
    // quota 只对当前 Auth Files 页生效，AI Provider 不参与缓存读取和刷新。
    () => selectQuotaEligibleAuthIndexes(credentialPages.authFileIdentities),
    [credentialPages.authFileIdentities],
  )
  const { quotaByAuthIndex, cachedQuotaStateByAuthIndex, setQuotaByAuthIndex } = useQuotaCache({
    enabled: enabledAuthFiles,
    authIndexes: currentAuthIndexes,
    onAuthRequired,
  })
  const quotaRefreshTasks = useQuotaRefreshTasks({
    enabled: enabledAuthFiles,
    currentAuthIndexes,
    setQuotaByAuthIndex,
    onAuthRequired,
  })

  // 把对象状态转成 Map 后交给纯 view model，组件层只消费已组合好的行数据。
  const quotaRowsByAuthIndex = useMemo(() => new Map(Object.entries(quotaByAuthIndex)), [quotaByAuthIndex])
  const quotaStates = useMemo(() => {
    const mergedStates = { ...cachedQuotaStateByAuthIndex, ...quotaRefreshTasks.quotaStateByAuthIndex }
    return new Map(Object.entries(mergedStates).map(([authIndex, state]) => [authIndex, {
      quotaLoading: state.loading ?? false,
      quotaError: state.error,
      refreshStatus: state.refreshStatus,
    }]))
  }, [cachedQuotaStateByAuthIndex, quotaRefreshTasks.quotaStateByAuthIndex])

  const authFileRows = useMemo(
    () => buildAuthFileCredentialRows(credentialPages.authFileIdentities, quotaRowsByAuthIndex, quotaStates),
    [credentialPages.authFileIdentities, quotaRowsByAuthIndex, quotaStates],
  )
  const aiProviderRows = useMemo(
    () => buildAiProviderCredentialRows(credentialPages.aiProviderIdentities),
    [credentialPages.aiProviderIdentities],
  )

  return {
    authFileRows,
    aiProviderRows,
    authFileTypeCounts: credentialPages.authFileTypeCounts,
    aiProviderTypeCounts: credentialPages.aiProviderTypeCounts,
    authFileTotal: credentialPages.authFileTotal,
    aiProviderTotal: credentialPages.aiProviderTotal,
    authFilePageSize: credentialPages.authFilePageSize,
    aiProviderPageSize: credentialPages.aiProviderPageSize,
    authFilePage: credentialPages.authFilePage,
    aiProviderPage: credentialPages.aiProviderPage,
    authFileTotalPages: credentialPages.authFileTotalPages,
    aiProviderTotalPages: credentialPages.aiProviderTotalPages,
    authFileActiveOnly: credentialPages.authFileActiveOnly,
    authFileProviderFilter: credentialPages.authFileProviderFilter,
    aiProviderProviderFilter: credentialPages.aiProviderProviderFilter,
    authFileSort: credentialPages.authFileSort,
    aiProviderSort: credentialPages.aiProviderSort,
    setAuthFilePage: credentialPages.setAuthFilePage,
    setAiProviderPage: credentialPages.setAiProviderPage,
    setAuthFilePageSize: credentialPages.setAuthFilePageSize,
    setAiProviderPageSize: credentialPages.setAiProviderPageSize,
    setAuthFileActiveOnly: credentialPages.setAuthFileActiveOnly,
    setAuthFileProviderFilter: credentialPages.setAuthFileProviderFilter,
    setAiProviderProviderFilter: credentialPages.setAiProviderProviderFilter,
    setAuthFileSort: credentialPages.setAuthFileSort,
    setAiProviderSort: credentialPages.setAiProviderSort,
    loading: credentialPages.loading,
    error: credentialPages.error,
    quotaRefreshing: quotaRefreshTasks.quotaRefreshing,
    quotaRefreshError: quotaRefreshTasks.quotaRefreshError,
    refresh: credentialPages.refresh,
    refreshQuotaForCurrentAuthFilePage: quotaRefreshTasks.refreshQuotaForCurrentAuthFilePage,
    refreshQuotaForAuthIndex: quotaRefreshTasks.refreshQuotaForAuthIndex,
  }
}

export { quotaRefreshDisplayError }
