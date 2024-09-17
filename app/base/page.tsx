'use client'

import { useState } from 'react'
import Datum from '@/components/datum'
import { generateAndRunQuery } from './actions'

export default function SQLGeneratorPage() {
  const [prompt, setPrompt] = useState('')
  const [result, setResult] = useState<any>(null)
  const [resultType, setResultType] = useState<'statistic' | 'table' | 'chart'>('table')
  const [error, setError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  const handleGenerateAndRunQuery = async () => {
    try {
      setIsLoading(true)
      setError(null)
      const { result, resultType } = await generateAndRunQuery(prompt)
      setResult(result)
      setResultType(resultType)
    } catch (error) {
      console.error('Error generating or running query:', error)
      setError('An error occurred while generating or running the query.')
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="container mx-auto">
      <form onSubmit={(e) => {
        e.preventDefault();
        handleGenerateAndRunQuery();
      }} className="flex space-x-2 mb-4">
        <input
          className="flex-grow px-3 py-2 border border-stone-300 rounded-md focus:outline-none focus:ring-2 focus:ring-stone-500"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="Query Kernbase using natural language..."
          autoFocus
        />
        <button
          type="submit"
          className="px-4 py-2 bg-stone-500 text-white rounded-md hover:bg-stone-600 focus:outline-none focus:ring-2 focus:ring-stone-500 disabled:opacity-50 disabled:cursor-not-allowed"
          disabled={isLoading}
        >
          {isLoading ? (
            <span className="flex items-center">
              <svg className="animate-spin -ml-1 mr-3 h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Running...
            </span>
          ) : (
            'Go'
          )}
        </button>
      </form>

      {error && (
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">
          <span className="block sm:inline">{error}</span>
        </div>
      )}

      {result && (
        <div className="py-6">
          {resultType === 'statistic' && (
            <div className="shadow-lg rounded-lg p-8 mb-6 border border-stone-300 max-w-fit mx-auto">
              <div className="text-4xl font-bold text-stone-800 text-center">
                <Datum value={Object.values(result[0])[0]} />
              </div>
            </div>
          )}
          {resultType === 'table' && (
            <div className="overflow-x-auto">
              <table className="w-full divide-y divide-stone-200">
                <thead className="bg-stone-50">
                  <tr>
                    {Object.keys(result[0]).map((key) => (
                      <th key={key} className="px-6 py-3 text-left text-xs font-medium text-stone-500 uppercase tracking-wider">
                        {key}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-stone-200">
                  {result.map((row: any, index: number) => (
                    <tr key={index}>
                      {Object.values(row).map((value: any, i: number) => (
                        <td key={i} className="px-6 py-4 whitespace-nowrap text-sm text-stone-500">
                          <Datum value={value} />
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {resultType === 'chart' && (
            <div className="h-64">
              {/* Implement your chart component here */}
              <p>Chart placeholder</p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
