import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Topbar } from '../components/Topbar'
import './AppDetail.css'

type TimeFilter = 'day' | 'week' | 'month'

export function AppDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('week')

  if (!id) {
    navigate('/apps')
    return null
  }

  return (
    <>
      <Topbar title="App Detail" timeFilter={timeFilter} onTimeFilter={setTimeFilter} />
      <div className="content">
        {/* App detail — counts, sparklines, event list — T-06+ */}
        <div className="app-detail-placeholder">
          <span className="app-detail-id">App {id}</span>
          <span>Detail view coming in T-06+</span>
        </div>
      </div>
    </>
  )
}
