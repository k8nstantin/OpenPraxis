import { Link } from 'react-router-dom';

// Placeholder home for the React v2 dashboard. Real chrome (header,
// nav, tab list) lands once enough sub-products migrate to justify
// rebuilding it; until then a single link to Products is enough to
// prove routing works end-to-end.
export default function Home() {
  return (
    <div className="page-home">
      <h1>OpenPraxis Dashboard v2</h1>
      <p>The React dashboard. Tabs migrate one at a time.</p>
      <ul>
        <li><Link to="/products">Products</Link></li>
        <li><a href="/">Legacy UI</a></li>
      </ul>
    </div>
  );
}
