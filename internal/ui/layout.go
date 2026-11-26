package ui

type viewLayout struct {
	workspaceHeight  int
	navigationWidth  int
	resourceHeight   int
	instanceHeight   int
	narrow           bool
	navigationHeight int
	resourceWidth    int
	instanceWidth    int
	detailWidth      int
	detailHeight     int
}

type paneBounds struct {
	panel  focus
	x      int
	y      int
	width  int
	height int
}

type paneSizes struct {
	workspaceHeight  int
	navigationWidth  int
	resourceHeight   int
	navigationHeight int
	resourceWidth    int
}

type divider int

const (
	noDivider divider = iota
	workspaceDivider
	navigationDivider
	resourceInstanceDivider
	narrowNavigationDivider
	narrowResourceInstanceDivider
)

const (
	minPaneExtent      = 3
	minNavigationWidth = 20
	minNarrowPaneWidth = 12
)

func (m *Model) layoutForView() viewLayout {
	paneAreaHeight := m.height - 2
	workspaceHeight := m.paneSizes.workspaceHeight
	if workspaceHeight == 0 {
		workspaceHeight = clamp(paneAreaHeight/5, 5, 7)
	}
	workspaceHeight = clamp(workspaceHeight, 5, paneAreaHeight-7)
	lowerHeight := paneAreaHeight - workspaceHeight
	layout := viewLayout{
		workspaceHeight: workspaceHeight,
		detailWidth:     m.width,
		detailHeight:    lowerHeight,
	}
	if m.detailOnly {
		return layout
	}
	if m.width >= 80 {
		layout.navigationWidth = m.paneSizes.navigationWidth
		if layout.navigationWidth == 0 {
			layout.navigationWidth = clamp(m.width*28/100, minNavigationWidth, 36)
		}
		layout.navigationWidth = clamp(layout.navigationWidth, minNavigationWidth, m.width-minNavigationWidth)
		layout.resourceHeight = m.paneSizes.resourceHeight
		if layout.resourceHeight == 0 {
			layout.resourceHeight = lowerHeight / 2
		}
		layout.resourceHeight = clamp(layout.resourceHeight, minPaneExtent, lowerHeight-minPaneExtent)
		layout.instanceHeight = lowerHeight - layout.resourceHeight
		layout.detailWidth = m.width - layout.navigationWidth
		return layout
	}
	layout.narrow = true
	layout.navigationHeight = m.paneSizes.navigationHeight
	if layout.navigationHeight == 0 {
		layout.navigationHeight = min(max(4, lowerHeight/3), lowerHeight-minPaneExtent)
	}
	layout.navigationHeight = clamp(layout.navigationHeight, minPaneExtent, lowerHeight-minPaneExtent)
	layout.resourceWidth = m.paneSizes.resourceWidth
	if layout.resourceWidth == 0 {
		layout.resourceWidth = m.width / 2
	}
	layout.resourceWidth = clamp(layout.resourceWidth, minNarrowPaneWidth, m.width-minNarrowPaneWidth)
	layout.instanceWidth = m.width - layout.resourceWidth
	layout.detailHeight = lowerHeight - layout.navigationHeight
	return layout
}

func (m *Model) paneVisibleRows(panel focus) int {
	layout := m.layoutForView()
	var height int
	switch panel {
	case workspaceFocus:
		height = layout.workspaceHeight
	case resourceFocus:
		if layout.narrow {
			height = layout.navigationHeight
		} else {
			height = layout.resourceHeight
		}
	case instanceFocus:
		if layout.narrow {
			height = layout.navigationHeight
		} else {
			height = layout.instanceHeight
		}
	case detailFocus:
		height = layout.detailHeight
	}
	return max(1, height-2)
}

func (m *Model) paneBoundsAt(x, y int) (paneBounds, bool) {
	if m.width < 40 || m.height < 16 {
		return paneBounds{}, false
	}
	layout := m.layoutForView()
	workspace := paneBounds{panel: workspaceFocus, x: 0, y: 1, width: m.width, height: layout.workspaceHeight}
	if containsPoint(workspace, x, y) {
		return workspace, true
	}

	lowerY := workspace.y + workspace.height
	if m.detailOnly {
		detail := paneBounds{panel: detailFocus, x: 0, y: lowerY, width: m.width, height: layout.detailHeight}
		if containsPoint(detail, x, y) {
			return detail, true
		}
		return paneBounds{}, false
	}
	if !layout.narrow {
		if containsPoint(paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.navigationWidth, height: layout.resourceHeight}, x, y) {
			return paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.navigationWidth, height: layout.resourceHeight}, true
		}
		instanceY := lowerY + layout.resourceHeight
		if containsPoint(paneBounds{panel: instanceFocus, x: 0, y: instanceY, width: layout.navigationWidth, height: layout.instanceHeight}, x, y) {
			return paneBounds{panel: instanceFocus, x: 0, y: instanceY, width: layout.navigationWidth, height: layout.instanceHeight}, true
		}
		detail := paneBounds{panel: detailFocus, x: layout.navigationWidth, y: lowerY, width: layout.detailWidth, height: layout.detailHeight}
		if containsPoint(detail, x, y) {
			return detail, true
		}
		return paneBounds{}, false
	}

	if containsPoint(paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.resourceWidth, height: layout.navigationHeight}, x, y) {
		return paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.resourceWidth, height: layout.navigationHeight}, true
	}
	if containsPoint(paneBounds{panel: instanceFocus, x: layout.resourceWidth, y: lowerY, width: layout.instanceWidth, height: layout.navigationHeight}, x, y) {
		return paneBounds{panel: instanceFocus, x: layout.resourceWidth, y: lowerY, width: layout.instanceWidth, height: layout.navigationHeight}, true
	}
	detailY := lowerY + layout.navigationHeight
	detail := paneBounds{panel: detailFocus, x: 0, y: detailY, width: layout.detailWidth, height: layout.detailHeight}
	if containsPoint(detail, x, y) {
		return detail, true
	}
	return paneBounds{}, false
}

func containsPoint(bounds paneBounds, x, y int) bool {
	return x >= bounds.x && x < bounds.x+bounds.width &&
		y >= bounds.y && y < bounds.y+bounds.height
}

func (m *Model) dividerAt(x, y int) (divider, bool) {
	if m.width < 40 || m.height < 16 {
		return noDivider, false
	}
	layout := m.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	if y == lowerY-1 {
		return workspaceDivider, true
	}
	if m.detailOnly {
		return noDivider, false
	}
	if !layout.narrow {
		if x == layout.navigationWidth-1 && y >= lowerY && y < lowerY+layout.detailHeight {
			return navigationDivider, true
		}
		if y == lowerY+layout.resourceHeight-1 && x >= 0 && x < layout.navigationWidth-1 {
			return resourceInstanceDivider, true
		}
		return noDivider, false
	}
	if y == lowerY+layout.navigationHeight-1 {
		return narrowNavigationDivider, true
	}
	if x == layout.resourceWidth-1 && y >= lowerY && y < lowerY+layout.navigationHeight-1 {
		return narrowResourceInstanceDivider, true
	}
	return noDivider, false
}

func (m *Model) beginResize(target divider, x, y int) {
	layout := m.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	boundary := 0
	switch target {
	case workspaceDivider:
		boundary = lowerY
	case navigationDivider:
		boundary = layout.navigationWidth
	case resourceInstanceDivider:
		boundary = lowerY + layout.resourceHeight
	case narrowNavigationDivider:
		boundary = lowerY + layout.navigationHeight
	case narrowResourceInstanceDivider:
		boundary = layout.resourceWidth
	}
	if target == navigationDivider || target == narrowResourceInstanceDivider {
		m.dragOffset = x - boundary
	} else {
		m.dragOffset = y - boundary
	}
	m.dragDivider = target
}

func (m *Model) resizeFrom(x, y int) {
	layout := m.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	switch m.dragDivider {
	case workspaceDivider:
		paneAreaHeight := m.height - 2
		m.paneSizes.workspaceHeight = clamp(y-m.dragOffset-1, 5, paneAreaHeight-7)
	case navigationDivider:
		m.paneSizes.navigationWidth = clamp(x-m.dragOffset, minNavigationWidth, m.width-minNavigationWidth)
	case resourceInstanceDivider:
		m.paneSizes.resourceHeight = clamp(y-m.dragOffset-lowerY, minPaneExtent, layout.detailHeight-minPaneExtent)
	case narrowNavigationDivider:
		lowerHeight := layout.navigationHeight + layout.detailHeight
		m.paneSizes.navigationHeight = clamp(y-m.dragOffset-lowerY, minPaneExtent, lowerHeight-minPaneExtent)
	case narrowResourceInstanceDivider:
		m.paneSizes.resourceWidth = clamp(x-m.dragOffset, minNarrowPaneWidth, m.width-minNarrowPaneWidth)
	}
}
