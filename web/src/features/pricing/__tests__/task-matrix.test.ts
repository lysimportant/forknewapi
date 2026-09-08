/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'

import { describe, test } from 'vitest'

import { parseTaskTiersFromExpr } from '../lib/billing-expr'
import {
  createDefaultTaskMatrixConfig,
  createDefaultTaskVisualConfig,
  evaluateTaskVisualConfig,
  generateTaskExprFromConfig,
  getTaskEnumCombinations,
  taskMatrixRowLabel,
  taskMatrixToTiers,
  tryParseTaskMatrixConfig,
  type TaskMatrixConfig,
} from '../lib/task-expr'
import type { BillingUsageSchema } from '../types'

const singleEnumSchema: BillingUsageSchema = {
  seconds: { type: 'number', unit: 'second' },
  mode: { enum: ['std', 'pro'] },
}

const doubleEnumSchema: BillingUsageSchema = {
  quality: { enum: ['high', 'low'] },
  seconds: { type: 'number', unit: 'second' },
  mode: { enum: ['std', 'pro'] },
}

const numberOnlySchema: BillingUsageSchema = {
  seconds: { type: 'number', unit: 'second' },
}

function createNonUniformMatrix(): TaskMatrixConfig {
  return {
    rows: [
      {
        combination: { mode: 'std', quality: 'high' },
        constant: 0.1,
        unitPrices: { seconds: 0.2 },
      },
      {
        combination: { mode: 'std', quality: 'low' },
        constant: 0.2,
        unitPrices: { seconds: 0.3 },
      },
      {
        combination: { mode: 'pro', quality: 'high' },
        constant: 0.3,
        unitPrices: { seconds: 0.4 },
      },
      {
        combination: { mode: 'pro', quality: 'low' },
        constant: 0.4,
        unitPrices: { seconds: 0.5 },
      },
    ],
  }
}

describe('task matrix enum combinations', () => {
  test('enumerates fields lexicographically and values in declaration order with the first field slowest', () => {
    assert.deepEqual(getTaskEnumCombinations(doubleEnumSchema), [
      { mode: 'std', quality: 'high' },
      { mode: 'std', quality: 'low' },
      { mode: 'pro', quality: 'high' },
      { mode: 'pro', quality: 'low' },
    ])
  })

  test('returns one empty combination for a number-only schema', () => {
    assert.deepEqual(getTaskEnumCombinations(numberOnlySchema), [{}])
  })
})

describe('uniform task matrix conversion', () => {
  test('collapses rows with identical prices into one unconditioned base tier', () => {
    const config: TaskMatrixConfig = {
      rows: getTaskEnumCombinations(doubleEnumSchema).map((combination) => ({
        combination,
        constant: 0.1,
        unitPrices: { seconds: 0.4 },
      })),
    }

    assert.deepEqual(taskMatrixToTiers(config, doubleEnumSchema), [
      {
        label: 'base',
        conditions: [],
        constant: 0.1,
        unitPrices: { seconds: 0.4 },
      },
    ])
  })

  test('generates the same default expression as the existing visual config', () => {
    const matrixExpression = generateTaskExprFromConfig(
      {
        tiers: taskMatrixToTiers(
          createDefaultTaskMatrixConfig(singleEnumSchema),
          singleEnumSchema
        ),
      },
      singleEnumSchema
    )
    const visualExpression = generateTaskExprFromConfig(
      createDefaultTaskVisualConfig(singleEnumSchema),
      singleEnumSchema
    )

    assert.equal(matrixExpression, visualExpression)
  })
})

describe('non-uniform task matrix conversion', () => {
  test('expands canonical rows with full ordered conditions and the final row as else', () => {
    assert.deepEqual(
      taskMatrixToTiers(createNonUniformMatrix(), doubleEnumSchema),
      [
        {
          label: 'std·high',
          conditions: [
            { field: 'mode', value: 'std' },
            { field: 'quality', value: 'high' },
          ],
          constant: 0.1,
          unitPrices: { seconds: 0.2 },
        },
        {
          label: 'std·low',
          conditions: [
            { field: 'mode', value: 'std' },
            { field: 'quality', value: 'low' },
          ],
          constant: 0.2,
          unitPrices: { seconds: 0.3 },
        },
        {
          label: 'pro·high',
          conditions: [
            { field: 'mode', value: 'pro' },
            { field: 'quality', value: 'high' },
          ],
          constant: 0.3,
          unitPrices: { seconds: 0.4 },
        },
        {
          label: 'pro·low',
          conditions: [],
          constant: 0.4,
          unitPrices: { seconds: 0.5 },
        },
      ]
    )
    assert.equal(taskMatrixRowLabel({ quality: 'low', mode: 'pro' }), 'pro·low')
    assert.equal(taskMatrixRowLabel({}), 'base')
  })
})

describe('task matrix round trips', () => {
  test('opens a legacy two-dimensional video matrix after size expansion without changing charges on save', () => {
    /** 旧视频价格覆盖两个模式和两个尺寸，最后一个组合使用兜底价。 */
    const legacySchema: BillingUsageSchema = {
      seconds: { type: 'number', unit: 'second' },
      mode: { enum: ['std', 'pro'] },
      size: { enum: ['720p', '1080p'] },
    }
    /** 扩展尺寸后，两个模式下的新组合都必须继承原兜底价。 */
    const expandedSchema: BillingUsageSchema = {
      ...legacySchema,
      size: { enum: ['720p', '1080p', '4k'] },
    }
    const expression =
      'u("size") == "720p" && u("mode") == "pro" ? tier("pro-small", 0.3 + u("seconds") * 0.4) : u("mode") == "std" && u("size") == "1080p" ? tier("std-large", 0.2 + u("seconds") * 0.3) : u("mode") == "std" && u("size") == "720p" ? tier("std-small", 0.1 + u("seconds") * 0.2) : tier("fallback", 0.4 + u("seconds") * 0.5)'
    const legacyTiers = parseTaskTiersFromExpr(expression, legacySchema)
    assert.equal(legacyTiers.length, 4)
    const matrix = tryParseTaskMatrixConfig(expression, expandedSchema)
    assert.ok(matrix)
    assert.deepEqual(matrix.rows, [
      {
        combination: { mode: 'std', size: '720p' },
        constant: 0.1,
        unitPrices: { seconds: 0.2 },
      },
      {
        combination: { mode: 'std', size: '1080p' },
        constant: 0.2,
        unitPrices: { seconds: 0.3 },
      },
      {
        combination: { mode: 'std', size: '4k' },
        constant: 0.4,
        unitPrices: { seconds: 0.5 },
      },
      {
        combination: { mode: 'pro', size: '720p' },
        constant: 0.3,
        unitPrices: { seconds: 0.4 },
      },
      {
        combination: { mode: 'pro', size: '1080p' },
        constant: 0.4,
        unitPrices: { seconds: 0.5 },
      },
      {
        combination: { mode: 'pro', size: '4k' },
        constant: 0.4,
        unitPrices: { seconds: 0.5 },
      },
    ])
    const savedExpression = generateTaskExprFromConfig(
      { tiers: taskMatrixToTiers(matrix, expandedSchema) },
      expandedSchema
    )
    assert.deepEqual(
      tryParseTaskMatrixConfig(savedExpression, expandedSchema),
      matrix
    )
    const savedTiers = parseTaskTiersFromExpr(savedExpression, expandedSchema)
    assert.equal(savedTiers.length, 6)
    for (const row of matrix.rows) {
      for (const seconds of [0, 3, 3600]) {
        const sample = { ...row.combination, seconds }
        const original = evaluateTaskVisualConfig(
          { tiers: legacyTiers },
          sample
        )
        const saved = evaluateTaskVisualConfig({ tiers: savedTiers }, sample)
        assert.ok(original)
        assert.ok(saved)
        assert.equal(saved.total, original.total)
        assert.equal(
          saved.total,
          row.constant + seconds * row.unitPrices.seconds
        )
      }
    }
    // schema 外的尺寸仍进入旧兜底，保存不能将它改成某个显式组合的价格。
    for (const mode of ['std', 'pro']) {
      for (const size of ['unknown-size', '']) {
        for (const seconds of [0, 3, 3600]) {
          const sample = { mode, size, seconds }
          const original = evaluateTaskVisualConfig(
            { tiers: legacyTiers },
            sample
          )
          const saved = evaluateTaskVisualConfig({ tiers: savedTiers }, sample)
          assert.ok(original)
          assert.ok(saved)
          assert.equal(original.total, 0.4 + seconds * 0.5)
          assert.equal(saved.total, original.total)
        }
      }
    }
  })

  test('preserves a uniform matrix through expression generation and recognition', () => {
    const config: TaskMatrixConfig = {
      rows: getTaskEnumCombinations(doubleEnumSchema).map((combination) => ({
        combination,
        constant: 0.1,
        unitPrices: { seconds: 0.4 },
      })),
    }
    const expression = generateTaskExprFromConfig(
      { tiers: taskMatrixToTiers(config, doubleEnumSchema) },
      doubleEnumSchema
    )

    assert.deepEqual(
      tryParseTaskMatrixConfig(expression, doubleEnumSchema),
      config
    )
  })

  test('preserves a non-uniform matrix through expression generation and recognition', () => {
    const config = createNonUniformMatrix()
    const expression = generateTaskExprFromConfig(
      { tiers: taskMatrixToTiers(config, doubleEnumSchema) },
      doubleEnumSchema
    )

    assert.deepEqual(
      tryParseTaskMatrixConfig(expression, doubleEnumSchema),
      config
    )
  })
})

describe('permuted complete task partitions', () => {
  test('recognizes legacy tier order, assigns prices canonically, and ignores labels', () => {
    const expression =
      'u("mode") == "pro" ? tier("legacy-pro", u("seconds") * 0.8) : tier("legacy-std", u("seconds") * 0.4)'
    const matrix = tryParseTaskMatrixConfig(expression, singleEnumSchema)

    assert.deepEqual(matrix, {
      rows: [
        {
          combination: { mode: 'std' },
          constant: 0,
          unitPrices: { seconds: 0.4 },
        },
        {
          combination: { mode: 'pro' },
          constant: 0,
          unitPrices: { seconds: 0.8 },
        },
      ],
    })
    assert.ok(matrix)
    const normalizedExpression = generateTaskExprFromConfig(
      { tiers: taskMatrixToTiers(matrix, singleEnumSchema) },
      singleEnumSchema
    )
    assert.deepEqual(
      tryParseTaskMatrixConfig(normalizedExpression, singleEnumSchema),
      matrix
    )
  })
})

describe('flat task expression recognition', () => {
  test('expands one flat tier across every enum combination', () => {
    assert.deepEqual(
      tryParseTaskMatrixConfig(
        'tier("base", u("seconds") * 0.4)',
        singleEnumSchema
      ),
      {
        rows: [
          {
            combination: { mode: 'std' },
            constant: 0,
            unitPrices: { seconds: 0.4 },
          },
          {
            combination: { mode: 'pro' },
            constant: 0,
            unitPrices: { seconds: 0.4 },
          },
        ],
      }
    )
  })

  test('recognizes one flat tier as the number-only matrix row', () => {
    assert.deepEqual(
      tryParseTaskMatrixConfig(
        'tier("base", u("seconds") * 0.4)',
        numberOnlySchema
      ),
      {
        rows: [
          {
            combination: {},
            constant: 0,
            unitPrices: { seconds: 0.4 },
          },
        ],
      }
    )
  })
})

describe('task matrix recognition rejection matrix', () => {
  test.each([
    ['partial conditions', 'u("mode") == "std"'],
    ['duplicate fields', 'u("mode") == "std" && u("mode") == "pro"'],
    ['unknown fields', 'u("unknown") == "std" && u("size") == "720p"'],
    ['unknown values', 'u("mode") == "std" && u("size") == "unknown"'],
    ['non-equality conditions', 'u("mode") != "std" && u("size") == "720p"'],
    ['alternative conditions', 'u("mode") == "std" || u("size") == "720p"'],
  ])(
    'rejects %s even when multiple combinations could use fallback',
    (_name, condition) => {
      /** 稀疏二维 schema 确保无效结构不会仅因档位数量被拒绝。 */
      const schema: BillingUsageSchema = {
        seconds: { type: 'number', unit: 'second' },
        mode: { enum: ['std', 'pro'] },
        size: { enum: ['720p', '1080p', '4k'] },
      }
      assert.equal(
        tryParseTaskMatrixConfig(
          `${condition} ? tier("explicit", u("seconds") * 0.2) : tier("fallback", u("seconds") * 0.5)`,
          schema
        ),
        null
      )
    }
  )

  test('rejects expressions outside the task tier grammar', () => {
    assert.equal(
      tryParseTaskMatrixConfig('u("seconds") * 0.4', singleEnumSchema),
      null
    )
    assert.equal(
      tryParseTaskMatrixConfig(
        'u("seconds") > 30 ? tier("long", u("seconds") * 0.3) : tier("short", u("seconds") * 0.4)',
        singleEnumSchema
      ),
      null
    )
    assert.equal(
      tryParseTaskMatrixConfig(
        'price("base", u("seconds") * 0.4)',
        singleEnumSchema
      ),
      null
    )
  })

  test('rejects undeclared usage fields and enum values', () => {
    assert.equal(
      tryParseTaskMatrixConfig(
        'tier("base", u("unknown") * 0.4)',
        singleEnumSchema
      ),
      null
    )
    assert.equal(
      tryParseTaskMatrixConfig(
        'u("mode") == "ultra" ? tier("ultra", u("seconds") * 0.8) : tier("base", u("seconds") * 0.4)',
        singleEnumSchema
      ),
      null
    )
  })

  test('rejects a tier condition that omits an enum field', () => {
    const schema: BillingUsageSchema = {
      seconds: { type: 'number', unit: 'second' },
      mode: { enum: ['std', 'pro'] },
      quality: { enum: ['high'] },
    }
    const expression =
      'u("mode") == "std" ? tier("std", u("seconds") * 0.4) : tier("pro", u("seconds") * 0.8)'

    assert.equal(tryParseTaskMatrixConfig(expression, schema), null)
  })

  test('rejects duplicate conditions for one enum field in a tier', () => {
    const schema: BillingUsageSchema = {
      seconds: { type: 'number', unit: 'second' },
      mode: { enum: ['std', 'pro'] },
      quality: { enum: ['high'] },
    }
    const expression =
      'u("mode") == "std" && u("mode") == "pro" ? tier("std", u("seconds") * 0.4) : tier("pro", u("seconds") * 0.8)'

    assert.equal(tryParseTaskMatrixConfig(expression, schema), null)
  })

  test('rejects duplicate combinations across tiers', () => {
    const schema: BillingUsageSchema = {
      seconds: { type: 'number', unit: 'second' },
      mode: { enum: ['std', 'pro', 'ultra', 'new'] },
    }
    const expression =
      'u("mode") == "std" ? tier("one", u("seconds") * 0.4) : u("mode") == "std" ? tier("two", u("seconds") * 0.6) : tier("base", u("seconds") * 0.8)'

    assert.equal(tryParseTaskMatrixConfig(expression, schema), null)
  })

  test('assigns uncovered enum values the fallback price and rejects an unreachable fallback', () => {
    const threeValueSchema: BillingUsageSchema = {
      seconds: { type: 'number', unit: 'second' },
      mode: { enum: ['std', 'pro', 'ultra'] },
    }
    const partialExpression =
      'u("mode") == "std" ? tier("std", u("seconds") * 0.4) : tier("base", u("seconds") * 0.8)'
    const excessExpression =
      'u("mode") == "std" ? tier("std", u("seconds") * 0.4) : u("mode") == "pro" ? tier("pro", u("seconds") * 0.6) : tier("extra", u("seconds") * 0.8)'

    assert.deepEqual(
      tryParseTaskMatrixConfig(partialExpression, threeValueSchema),
      {
        rows: [
          {
            combination: { mode: 'std' },
            constant: 0,
            unitPrices: { seconds: 0.4 },
          },
          {
            combination: { mode: 'pro' },
            constant: 0,
            unitPrices: { seconds: 0.8 },
          },
          {
            combination: { mode: 'ultra' },
            constant: 0,
            unitPrices: { seconds: 0.8 },
          },
        ],
      }
    )
    assert.equal(
      tryParseTaskMatrixConfig(excessExpression, singleEnumSchema),
      null
    )
  })

  test('rejects a conditional expression without an unconditioned final tier', () => {
    const expression = 'u("mode") == "std" ? tier("std", u("seconds") * 0.4)'

    assert.equal(tryParseTaskMatrixConfig(expression, singleEnumSchema), null)
  })

  test('rejects multiple tiers when the schema has no enum fields', () => {
    const expression =
      'u("mode") == "std" ? tier("std", u("seconds") * 0.4) : tier("base", u("seconds") * 0.8)'

    assert.equal(tryParseTaskMatrixConfig(expression, numberOnlySchema), null)
  })
})

describe('task matrix grammar cross-check', () => {
  test('bills the same total and tier label as the highlighted matrix row', () => {
    const matrix = createNonUniformMatrix()
    const tiers = taskMatrixToTiers(matrix, doubleEnumSchema)
    const expression = generateTaskExprFromConfig({ tiers }, doubleEnumSchema)
    const grammarTiers = parseTaskTiersFromExpr(expression, doubleEnumSchema)
    const combinations = getTaskEnumCombinations(doubleEnumSchema)

    for (const [matchedRowIndex, combination] of combinations.entries()) {
      const sample: Record<string, number | string> = {
        ...combination,
        seconds: 3,
      }
      const result = evaluateTaskVisualConfig({ tiers }, sample)
      const grammarTier =
        grammarTiers
          .slice(0, -1)
          .find((tier) =>
            tier.conditions.every(
              (condition) => sample[condition.field] === condition.value
            )
          ) ?? grammarTiers.at(-1)
      assert.ok(result)
      assert.ok(grammarTier)
      const grammarTotal =
        grammarTier.constant +
        Object.entries(grammarTier.unitPrices).reduce(
          (total, [field, unitPrice]) =>
            total + Number(sample[field]) * unitPrice,
          0
        )

      assert.equal(result.total, grammarTotal)
      assert.equal(result.tier.label, grammarTier.label)
      assert.equal(
        grammarTier.label,
        taskMatrixRowLabel(combinations[matchedRowIndex])
      )
    }
  })
})

describe('uniform task matrix preview highlighting', () => {
  test('keeps the sampled combination row identifiable after tiers collapse', () => {
    const matrix: TaskMatrixConfig = {
      rows: getTaskEnumCombinations(doubleEnumSchema).map((combination) => ({
        combination,
        constant: 0.1,
        unitPrices: { seconds: 0.4 },
      })),
    }
    const tiers = taskMatrixToTiers(matrix, doubleEnumSchema)
    const sample: Record<string, number | string> = {
      mode: 'pro',
      quality: 'low',
      seconds: 3,
    }
    const combinations = getTaskEnumCombinations(doubleEnumSchema)
    const matchedRowIndex = combinations.findIndex((combination) =>
      Object.entries(combination).every(
        ([field, value]) => sample[field] === value
      )
    )
    const result = evaluateTaskVisualConfig({ tiers }, sample)

    assert.ok(result)
    assert.equal(result.total, 0.1 + 3 * 0.4)
    assert.equal(result.tier.label, 'base')
    assert.equal(matchedRowIndex, 3)
    assert.equal(taskMatrixRowLabel(combinations[matchedRowIndex]), 'pro·low')
  })
})
