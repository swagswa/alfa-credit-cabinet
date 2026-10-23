import { useLocation, useParams } from 'react-router';
import { Applications } from './Applications';
import { Dashboard } from './Dashboard';
export function ClientCabinet() {
 const {id}=useParams();const {pathname}=useLocation();
 if(id)return <Applications />;
 if(pathname==='/client/new')return <Applications createOnly />;
 if(pathname==='/client/applications')return <Applications />;
 return <Dashboard />;
}
