import type { UsageComparisonItem } from '@/lib/types';

export interface ComparisonRow extends UsageComparisonItem {
  value: number;
  share: number | null;
  other?: boolean;
}
export interface ComparisonRect {
  row: ComparisonRow;
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface TreemapLayoutOptions {
  widthPx?: number;
  heightPx?: number;
  minWidthPx?: number;
  minHeightPx?: number;
}

export const COMPARISON_TOP_LIMIT = 5;
const SUM_FIELDS = ['requests', 'failures', 'input_tokens', 'output_tokens', 'cache_read_tokens', 'cache_creation_tokens', 'reasoning_tokens', 'total_tokens'] as const;

export function buildComparisonView(items: UsageComparisonItem[], othersLabel: string) {
  const ranked = [...items].sort((a,b) => b.total_tokens - a.total_tokens || a.key.localeCompare(b.key));
  const total = ranked.reduce((sum,item) => sum + item.total_tokens,0);
  const rows: ComparisonRow[] = ranked.slice(0,COMPARISON_TOP_LIMIT).map(item => ({
    ...item, value:item.total_tokens, share:total > 0 ? item.total_tokens / total * 100 : null,
  }));
  // 排名与面积固定按 Token；“其他”同时合并 Tooltip/列表指标，任何缺价都保留为未知。
  const tail = ranked.slice(COMPARISON_TOP_LIMIT);
  if (tail.length > 0) {
    const other: ComparisonRow = {
      key:'__comparison_others__', label:othersLabel, other:true, value:0, share:null,
      requests:0, failures:0, input_tokens:0, output_tokens:0, cache_read_tokens:0,
      cache_creation_tokens:0, reasoning_tokens:0, total_tokens:0, cost:0,
    };
    for (const item of tail) {
      for (const field of SUM_FIELDS) other[field] += item[field];
      other.cost = other.cost === null || item.cost === null ? null : other.cost + item.cost;
    }
    other.value = other.total_tokens;
    other.share = total > 0 ? other.total_tokens / total * 100 : null;
    rows.push(other);
  }
  return {rows,total};
}

type LayoutItem = { row: ComparisonRow; area: number };

const sumAreas = (items: LayoutItem[]) => items.reduce((sum, item) => sum + item.area, 0);

const worstAspect = (items: LayoutItem[], side: number) => {
  if (items.length === 0 || side <= 0) return Number.POSITIVE_INFINITY;
  const total = sumAreas(items);
  const max = Math.max(...items.map(item => item.area));
  const min = Math.min(...items.map(item => item.area));
  if (min <= 0 || total <= 0) return Number.POSITIVE_INFINITY;
  return Math.max((side * side * max) / (total * total), (total * total) / (side * side * min));
};

function layoutSquarified(items: LayoutItem[], x: number, y: number, width: number, height: number, result: ComparisonRect[]) {
  if (items.length === 0 || width <= 0 || height <= 0) return;
  if (items.length === 1) {
    result.push({row: items[0].row, x, y, width, height});
    return;
  }

  const side = Math.min(width, height);
  const row: LayoutItem[] = [items[0]];
  let consumed = 1;
  while (consumed < items.length) {
    const candidate = items[consumed];
    if (worstAspect([...row, candidate], side) <= worstAspect(row, side)) {
      row.push(candidate);
      consumed += 1;
      continue;
    }
    break;
  }

  const rowArea = sumAreas(row);
  if (width >= height) {
    const rowWidth = rowArea / height;
    let cursor = y;
    for (const item of row) {
      const itemHeight = item.area / rowWidth;
      result.push({row: item.row, x, y: cursor, width: rowWidth, height: itemHeight});
      cursor += itemHeight;
    }
    layoutSquarified(items.slice(consumed), x + rowWidth, y, width - rowWidth, height, result);
  } else {
    const rowHeight = rowArea / width;
    let cursor = x;
    for (const item of row) {
      const itemWidth = item.area / rowHeight;
      result.push({row: item.row, x: cursor, y, width: itemWidth, height: rowHeight});
      cursor += itemWidth;
    }
    layoutSquarified(items.slice(consumed), x, y + rowHeight, width, height - rowHeight, result);
  }
}

function normalizedRects(rects: ComparisonRect[], width: number, height: number): ComparisonRect[] {
  return rects.map(rect => ({
    ...rect,
    x: rect.x / width * 100,
    y: rect.y / height * 100,
    width: rect.width / width * 100,
    height: rect.height / height * 100,
  }));
}

function baseTreemap(rows: ComparisonRow[], width: number, height: number): ComparisonRect[] {
  const positive = rows.filter(row => row.value > 0);
  const total = positive.reduce((sum, row) => sum + row.value, 0);
  if (positive.length === 0 || total <= 0) return [];
  const items = positive.map(row => ({row, area: row.value / total * width * height}));
  const result: ComparisonRect[] = [];
  layoutSquarified(items, 0, 0, width, height, result);
  return result;
}

interface LayoutBox { x: number; y: number; width: number; height: number }
interface ConstrainedLayout { rects: ComparisonRect[]; score: number }

function constrainedLayoutScore(rects: ComparisonRect[], items: LayoutItem[], box: LayoutBox): number {
  const totalArea = sumAreas(items);
  const rowAreas = new Map(items.map(item => [item.row.key, item.area]));
  return rects.reduce((score, rect) => {
    const aspect = Math.max(rect.width / rect.height, rect.height / rect.width);
    const actualShare = rect.width * rect.height / (box.width * box.height);
    const targetShare = (rowAreas.get(rect.row.key) ?? 0) / totalArea;
    const distortion = Math.abs(Math.log(Math.max(actualShare, 1e-9) / Math.max(targetShare, 1e-9)));
    return score + Math.log(aspect) ** 2 + distortion * 3;
  }, 0);
}

function enumeratePartitions(items: LayoutItem[]): LayoutItem[][][] {
  const partitions: LayoutItem[][][] = [];
  const visit = (start: number, groups: LayoutItem[][]) => {
    if (start === items.length) {
      partitions.push(groups.map(group => [...group]));
      return;
    }
    for (let end = start + 1; end <= items.length; end += 1) {
      groups.push(items.slice(start, end));
      visit(end, groups);
      groups.pop();
    }
  };
  visit(0, []);
  return partitions;
}

function allocateLengths(weights: number[], totalLength: number, minimum: number): number[] | null {
  if (weights.length === 0 || weights.length * minimum > totalLength + 1e-6) return null;
  const remaining = Math.max(0, totalLength - weights.length * minimum);
  const totalWeight = weights.reduce((sum, weight) => sum + weight, 0);
  return weights.map(weight => minimum + remaining * weight / totalWeight);
}

function searchConstrainedLayout(items: LayoutItem[], box: LayoutBox, minWidth: number, minHeight: number): ConstrainedLayout | null {
  let best: ConstrainedLayout | null = null;
  for (const groups of enumeratePartitions(items)) {
    const groupAreas = groups.map(sumAreas);
    const columnWidths = allocateLengths(groupAreas, box.width, minWidth);
    if (columnWidths && groups.every(group => group.length * minHeight <= box.height + 1e-6)) {
      const rects: ComparisonRect[] = [];
      let x = box.x;
      for (const [groupIndex, group] of groups.entries()) {
        const heights = allocateLengths(group.map(item => item.area), box.height, minHeight)!;
        let y = box.y;
        for (const [itemIndex, item] of group.entries()) {
          rects.push({row: item.row, x, y, width: columnWidths[groupIndex], height: heights[itemIndex]});
          y += heights[itemIndex];
        }
        x += columnWidths[groupIndex];
      }
      const score = constrainedLayoutScore(rects, items, box);
      if (!best || score < best.score) best = {rects, score};
    }

    const rowHeights = allocateLengths(groupAreas, box.height, minHeight);
    if (rowHeights && groups.every(group => group.length * minWidth <= box.width + 1e-6)) {
      const rects: ComparisonRect[] = [];
      let y = box.y;
      for (const [groupIndex, group] of groups.entries()) {
        const widths = allocateLengths(group.map(item => item.area), box.width, minWidth)!;
        let x = box.x;
        for (const [itemIndex, item] of group.entries()) {
          rects.push({row: item.row, x, y, width: widths[itemIndex], height: rowHeights[groupIndex]});
          x += widths[itemIndex];
        }
        y += rowHeights[groupIndex];
      }
      const score = constrainedLayoutScore(rects, items, box);
      if (!best || score < best.score) best = {rects, score};
    }
  }
  return best;
}

function layoutGridFallback(items: LayoutItem[], width: number, height: number, minWidth: number, minHeight: number): ComparisonRect[] | null {
  let bestColumns = 0;
  let bestScore = Number.POSITIVE_INFINITY;
  for (let columns = 1; columns <= items.length; columns += 1) {
    const rows = Math.ceil(items.length / columns);
    const cellWidth = width / columns;
    const cellHeight = height / rows;
    if (cellWidth < minWidth || cellHeight < minHeight) continue;
    const score = Math.abs(Math.log(cellWidth / cellHeight));
    if (score < bestScore) {
      bestColumns = columns;
      bestScore = score;
    }
  }
  if (bestColumns === 0) return null;
  const rowCount = Math.ceil(items.length / bestColumns);
  const cellHeight = height / rowCount;
  return items.map((item, index) => {
    const row = Math.floor(index / bestColumns);
    const rowStart = row * bestColumns;
    const rowItems = Math.min(bestColumns, items.length - rowStart);
    const cellWidth = width / rowItems;
    const column = index - rowStart;
    return {row: item.row, x: column * cellWidth, y: row * cellHeight, width: cellWidth, height: cellHeight};
  });
}

function rankOrderedRects(rects: ComparisonRect[], rows: ComparisonRow[]): ComparisonRect[] {
  const rank = new Map(rows.map((row, index) => [row.key, index]));
  return [...rects].sort((left, right) => (rank.get(left.row.key) ?? Number.MAX_SAFE_INTEGER) - (rank.get(right.row.key) ?? Number.MAX_SAFE_INTEGER));
}

function applyVisibilityFloor(rows: ComparisonRow[], base: ComparisonRect[], width: number, height: number, minWidth: number, minHeight: number): ComparisonRect[] {
  if (!base.some(rect => rect.width < minWidth || rect.height < minHeight)) return normalizedRects(base, width, height);
  const positive = rows.filter(row => row.value > 0);
  const total = positive.reduce((sum, row) => sum + row.value, 0);
  const tailSlots = Math.max(1, positive.length - 1);
  const minimumArea = Math.max(minWidth * minHeight, minWidth * height / tailSlots, minHeight * width / tailSlots);
  const adjusted = positive.map(row => ({row, area: Math.max(row.value / total * width * height, minimumArea)}));
  const adjustedTotal = sumAreas(adjusted);
  const result: ComparisonRect[] = [];
  const normalizedItems = adjusted.map(item => ({...item, area: item.area / adjustedTotal * width * height}));
  layoutSquarified(normalizedItems, 0, 0, width, height, result);
  if (result.every(rect => rect.width >= minWidth && rect.height >= minHeight)) {
    return normalizedRects(rankOrderedRects(result, rows), width, height);
  }
  const constrained = searchConstrainedLayout(normalizedItems, {x: 0, y: 0, width, height}, minWidth, minHeight)?.rects
    ?? layoutGridFallback(normalizedItems, width, height, minWidth, minHeight)
    ?? result;
  return normalizedRects(rankOrderedRects(constrained, rows), width, height);
}

// Squarified layout keeps dominant values visually prominent while the optional visibility floor reserves readable compact cells.
export function layoutComparisonTreemap(rows: ComparisonRow[], aspect = 1, options: TreemapLayoutOptions = {}): ComparisonRect[] {
  const width = options.widthPx ?? Math.max(aspect, 0.01);
  const height = options.heightPx ?? 1;
  const base = baseTreemap(rows, width, height);
  if (options.minWidthPx === undefined || options.minHeightPx === undefined) {
    return normalizedRects(base, width, height);
  }
  return applyVisibilityFloor(rows, base, width, height, options.minWidthPx, options.minHeightPx);
}
