package parser

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type storageVersion struct {
	Major uint16
	Minor uint16
	Patch uint16
	Build uint16
}

func (v storageVersion) AtLeast(major uint16, minor uint16, patch uint16, build uint16) bool {
	left := [4]uint16{v.Major, v.Minor, v.Patch, v.Build}
	right := [4]uint16{major, minor, patch, build}
	for i := 0; i < len(left); i++ {
		if left[i] > right[i] {
			return true
		}
		if left[i] < right[i] {
			return false
		}
	}
	return true
}

type signalKind byte

const (
	signalItem signalKind = iota
	signalFluid
	signalVirtual
)

type prototypeIndex struct {
	items    map[uint16]string
	fluids   map[uint16]string
	virtuals map[uint16]string
}

func newPrototypeIndex() *prototypeIndex {
	return &prototypeIndex{
		items:    make(map[uint16]string),
		fluids:   make(map[uint16]string),
		virtuals: make(map[uint16]string),
	}
}

func (p *prototypeIndex) add(prototype string, id uint16, name string) {
	switch prototypeCategory(prototype) {
	case signalItem:
		p.items[id] = name
	case signalFluid:
		p.fluids[id] = name
	case signalVirtual:
		p.virtuals[id] = name
	}
}

func (p *prototypeIndex) resolve(kind signalKind, id uint16) string {
	switch kind {
	case signalItem:
		return p.items[id]
	case signalFluid:
		return p.fluids[id]
	case signalVirtual:
		return p.virtuals[id]
	default:
		return ""
	}
}

func prototypeCategory(prototype string) signalKind {
	switch prototype {
	case "fluid":
		return signalFluid
	case "virtual-signal":
		return signalVirtual
	default:
		return signalItem
	}
}

type libraryNode struct {
	Kind        string
	Slot        int
	Label       string
	Description string
	IconSprite  string
	Children    []libraryNode
}

type byteStream struct {
	data []byte
	pos  int
}

func newByteStream(data []byte) *byteStream {
	return &byteStream{data: data}
}

func (s *byteStream) position() int {
	return s.pos
}

func (s *byteStream) seek(position int) error {
	if position < 0 || position > len(s.data) {
		return fmt.Errorf("seek to invalid position %d", position)
	}
	s.pos = position
	return nil
}

func (s *byteStream) skip(count int) error {
	return s.seek(s.pos + count)
}

func (s *byteStream) readBytes(count int) ([]byte, error) {
	if count < 0 || s.pos+count > len(s.data) {
		return nil, fmt.Errorf("unexpected EOF at %d while reading %d bytes", s.pos, count)
	}
	out := s.data[s.pos : s.pos+count]
	s.pos += count
	return out, nil
}

func (s *byteStream) u8() (uint8, error) {
	bytes, err := s.readBytes(1)
	if err != nil {
		return 0, err
	}
	return bytes[0], nil
}

func (s *byteStream) u16() (uint16, error) {
	bytes, err := s.readBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(bytes), nil
}

func (s *byteStream) u32() (uint32, error) {
	bytes, err := s.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(bytes), nil
}

func (s *byteStream) s32() (int32, error) {
	value, err := s.u32()
	return int32(value), err
}

func (s *byteStream) bool() (bool, error) {
	value, err := s.u8()
	if err != nil {
		return false, err
	}
	if value != 0 && value != 1 {
		return false, fmt.Errorf("invalid bool %d at %d", value, s.pos-1)
	}
	return value == 1, nil
}

func (s *byteStream) count() (uint32, error) {
	length, err := s.u8()
	if err != nil {
		return 0, err
	}
	if length == 0xff {
		return s.u32()
	}
	return uint32(length), nil
}

func (s *byteStream) string() (string, error) {
	length, err := s.count()
	if err != nil {
		return "", err
	}
	bytes, err := s.readBytes(int(length))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *byteStream) expect(expected ...byte) error {
	for _, want := range expected {
		got, err := s.u8()
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("expected 0x%02x, got 0x%02x at %d", want, got, s.pos-1)
		}
	}
	return nil
}

func ParseBlueprintLibrary(data []byte) ([]Entry, error) {
	stream := newByteStream(data)

	version, err := parseVersion(stream)
	if err != nil {
		return nil, fmt.Errorf("read library version: %w", err)
	}
	if err := stream.expect(0x00); err != nil {
		return nil, fmt.Errorf("read library separator: %w", err)
	}
	if err := skipMigrations(stream); err != nil {
		return nil, fmt.Errorf("skip migrations: %w", err)
	}

	index, err := skipIndex(stream, version)
	if err != nil {
		return nil, fmt.Errorf("skip prototype index: %w", err)
	}

	if err := stream.skip(1); err != nil {
		return nil, fmt.Errorf("skip library state: %w", err)
	}
	if err := stream.expect(0x00); err != nil {
		return nil, fmt.Errorf("read library separator 2: %w", err)
	}
	if _, err := stream.u32(); err != nil {
		return nil, fmt.Errorf("read generation counter: %w", err)
	}
	if _, err := stream.u32(); err != nil {
		return nil, fmt.Errorf("read timestamp: %w", err)
	}
	if version.Major >= 2 {
		if _, err := stream.u32(); err != nil {
			return nil, fmt.Errorf("read library reserved field: %w", err)
		}
	}
	if err := stream.expect(0x01); err != nil {
		return nil, fmt.Errorf("read library object marker: %w", err)
	}

	nodes, err := parseLibraryObjects(stream, index, version)
	if err != nil {
		return nil, err
	}

	entries := flattenNodes(nodes)
	if len(entries) == 0 {
		return nil, errors.New("no blueprint or blueprint-book entries found")
	}

	return entries, nil
}

func parseVersion(stream *byteStream) (storageVersion, error) {
	major, err := stream.u16()
	if err != nil {
		return storageVersion{}, err
	}
	minor, err := stream.u16()
	if err != nil {
		return storageVersion{}, err
	}
	patch, err := stream.u16()
	if err != nil {
		return storageVersion{}, err
	}
	build, err := stream.u16()
	if err != nil {
		return storageVersion{}, err
	}
	return storageVersion{Major: major, Minor: minor, Patch: patch, Build: build}, nil
}

func skipMigrations(stream *byteStream) error {
	count, err := stream.u8()
	if err != nil {
		return err
	}
	for i := 0; i < int(count); i++ {
		if _, err := stream.string(); err != nil {
			return err
		}
		if _, err := stream.string(); err != nil {
			return err
		}
	}
	return nil
}

func skipIndex(stream *byteStream, version storageVersion) (*prototypeIndex, error) {
	index := newPrototypeIndex()

	prototypeCount, err := stream.u16()
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(prototypeCount); i++ {
		prototypeName, err := stream.string()
		if err != nil {
			return nil, err
		}

		useWideIDs := usesWidePrototypeIDs(version, prototypeName)
		var nameCount uint16
		if useWideIDs {
			nameCount, err = stream.u16()
		} else {
			count8, readErr := stream.u8()
			err = readErr
			nameCount = uint16(count8)
		}
		if err != nil {
			return nil, err
		}

		for j := 0; j < int(nameCount); j++ {
			var id uint16
			if useWideIDs {
				id, err = stream.u16()
			} else {
				id8, readErr := stream.u8()
				err = readErr
				id = uint16(id8)
			}
			if err != nil {
				return nil, err
			}

			name, err := stream.string()
			if err != nil {
				return nil, err
			}
			index.add(prototypeName, id, name)
		}
	}

	return index, nil
}

func usesWidePrototypeIDs(version storageVersion, prototypeName string) bool {
	if prototypeName == "quality" {
		return false
	}
	if version.Major >= 2 {
		return true
	}
	return prototypeName != "tile"
}

func parseLibraryObjects(stream *byteStream, index *prototypeIndex, version storageVersion) ([]libraryNode, error) {
	objectCount, err := stream.u32()
	if err != nil {
		return nil, err
	}

	nodes := make([]libraryNode, 0)
	for slot := 0; slot < int(objectCount); slot++ {
		used, err := stream.bool()
		if err != nil {
			return nil, fmt.Errorf("read library slot %d used flag: %w", slot+1, err)
		}
		if !used {
			continue
		}

		prefix, err := stream.u8()
		if err != nil {
			return nil, fmt.Errorf("read library slot %d prefix: %w", slot+1, err)
		}
		if _, err := stream.u32(); err != nil {
			return nil, fmt.Errorf("read library slot %d generation: %w", slot+1, err)
		}
		if _, err := stream.u16(); err != nil {
			return nil, fmt.Errorf("read library slot %d item id: %w", slot+1, err)
		}

		switch prefix {
		case 0:
			node, err := parseBlueprint(stream, version)
			if err != nil {
				return nil, fmt.Errorf("parse blueprint at slot %d: %w", slot+1, err)
			}
			node.Slot = slot + 1
			nodes = append(nodes, node)
		case 1:
			node, err := parseBlueprintBook(stream, index, version)
			if err != nil {
				return nil, fmt.Errorf("parse blueprint book at slot %d: %w", slot+1, err)
			}
			node.Slot = slot + 1
			nodes = append(nodes, node)
		case 2:
			if err := skipDeconstructionPlanner(stream, version); err != nil {
				return nil, fmt.Errorf("skip deconstruction planner at slot %d: %w", slot+1, err)
			}
		case 3:
			if err := skipUpgradePlanner(stream, version); err != nil {
				return nil, fmt.Errorf("skip upgrade planner at slot %d: %w", slot+1, err)
			}
		default:
			return nil, fmt.Errorf("unknown library object prefix %d at %d", prefix, stream.position()-1)
		}
	}

	return nodes, nil
}

func parseBlueprint(stream *byteStream, libraryVersion storageVersion) (libraryNode, error) {
	label, err := stream.string()
	if err != nil {
		return libraryNode{}, err
	}
	if err := stream.expect(0x00); err != nil {
		return libraryNode{}, err
	}
	hasRemovedMods, err := stream.bool()
	if err != nil {
		return libraryNode{}, err
	}
	contentSize, err := stream.count()
	if err != nil {
		return libraryNode{}, err
	}
	contentStart := stream.position()

	if _, err := parseVersion(stream); err != nil {
		return libraryNode{}, err
	}
	if err := stream.expect(0x00); err != nil {
		return libraryNode{}, err
	}
	if err := skipMigrations(stream); err != nil {
		return libraryNode{}, err
	}
	description, err := stream.string()
	if err != nil {
		return libraryNode{}, err
	}

	if err := stream.seek(contentStart + int(contentSize)); err != nil {
		return libraryNode{}, err
	}

	if hasRemovedMods {
		localIndexSize, err := stream.count()
		if err != nil {
			return libraryNode{}, err
		}
		if err := stream.skip(int(localIndexSize)); err != nil {
			return libraryNode{}, err
		}
	}

	return libraryNode{
		Kind:        "blueprint",
		Label:       label,
		Description: description,
	}, nil
}

func parseBlueprintBook(stream *byteStream, index *prototypeIndex, version storageVersion) (libraryNode, error) {
	label, err := stream.string()
	if err != nil {
		return libraryNode{}, err
	}
	description, err := stream.string()
	if err != nil {
		return libraryNode{}, err
	}
	iconSprite, err := parseIcons(stream, index, version)
	if err != nil {
		return libraryNode{}, err
	}
	children, err := parseLibraryObjects(stream, index, version)
	if err != nil {
		return libraryNode{}, err
	}
	if _, err := stream.u8(); err != nil {
		return libraryNode{}, err
	}
	if err := stream.expect(0x00); err != nil {
		return libraryNode{}, err
	}

	return libraryNode{
		Kind:        "blueprint-book",
		Label:       label,
		Description: description,
		IconSprite:  iconSprite,
		Children:    children,
	}, nil
}

func parseIcons(stream *byteStream, index *prototypeIndex, version storageVersion) (string, error) {
	unknownCount, err := stream.u8()
	if err != nil {
		return "", err
	}
	unknownNames := make([]string, 0, int(unknownCount))
	for i := 0; i < int(unknownCount); i++ {
		name, err := stream.string()
		if err != nil {
			return "", err
		}
		unknownNames = append(unknownNames, name)
	}

	iconCount, err := stream.u8()
	if err != nil {
		return "", err
	}

	var firstSprite string
	for i := 0; i < int(iconCount); i++ {
		typeID, err := stream.u8()
		if err != nil {
			return "", err
		}
		nameID, err := stream.u16()
		if err != nil {
			return "", err
		}
		if version.Major >= 2 {
			if _, err := stream.u8(); err != nil {
				return "", err
			}
		}

		if firstSprite != "" {
			continue
		}

		kind := signalKind(typeID)
		name := ""
		if i < len(unknownNames) && unknownNames[i] != "" {
			name = unknownNames[i]
		} else {
			name = index.resolve(kind, nameID)
		}
		if name == "" {
			continue
		}

		switch kind {
		case signalItem:
			firstSprite = "item/" + name
		case signalFluid:
			firstSprite = "fluid/" + name
		case signalVirtual:
			firstSprite = "virtual-signal/" + name
		}
	}

	return firstSprite, nil
}

func skipDeconstructionPlanner(stream *byteStream, version storageVersion) error {
	if _, err := stream.string(); err != nil {
		return err
	}
	if _, err := stream.string(); err != nil {
		return err
	}
	if _, err := parseIcons(stream, newPrototypeIndex(), version); err != nil {
		return err
	}
	if _, err := stream.u8(); err != nil {
		return err
	}
	if err := skipUnknownFilters(stream); err != nil {
		return err
	}
	if err := skipFilterEntries(stream, version, false); err != nil {
		return err
	}
	if _, err := stream.bool(); err != nil {
		return err
	}
	if _, err := stream.u8(); err != nil {
		return err
	}
	if _, err := stream.u8(); err != nil {
		return err
	}
	if err := skipUnknownFilters(stream); err != nil {
		return err
	}
	return skipFilterEntries(stream, version, true)
}

func skipUpgradePlanner(stream *byteStream, version storageVersion) error {
	if _, err := stream.string(); err != nil {
		return err
	}
	if _, err := stream.string(); err != nil {
		return err
	}
	if _, err := parseIcons(stream, newPrototypeIndex(), version); err != nil {
		return err
	}

	unknownCount, err := stream.u8()
	if err != nil {
		return err
	}
	for i := 0; i < int(unknownCount); i++ {
		if _, err := stream.string(); err != nil {
			return err
		}
		if _, err := stream.bool(); err != nil {
			return err
		}
		if _, err := stream.u16(); err != nil {
			return err
		}
	}

	mapperCount, err := stream.u8()
	if err != nil {
		return err
	}
	for i := 0; i < int(mapperCount); i++ {
		if err := skipUpgradeMapper(stream); err != nil {
			return err
		}
		if err := skipUpgradeMapper(stream); err != nil {
			return err
		}
	}

	return nil
}

func skipUnknownFilters(stream *byteStream) error {
	unknownCount, err := stream.u8()
	if err != nil {
		return err
	}
	for i := 0; i < int(unknownCount); i++ {
		if _, err := stream.u16(); err != nil {
			return err
		}
		if _, err := stream.string(); err != nil {
			return err
		}
	}
	return nil
}

func skipFilterEntries(stream *byteStream, version storageVersion, tile bool) error {
	filterCount, err := stream.u8()
	if err != nil {
		return err
	}
	for i := 0; i < int(filterCount); i++ {
		if tile && version.Major < 2 {
			if _, err := stream.u8(); err != nil {
				return err
			}
			continue
		}
		if _, err := stream.u16(); err != nil {
			return err
		}
		if version.Major >= 2 && !tile {
			if _, err := stream.u8(); err != nil {
				return err
			}
		}
	}
	return nil
}

func skipUpgradeMapper(stream *byteStream) error {
	if _, err := stream.u8(); err != nil {
		return err
	}
	_, err := stream.u16()
	return err
}

func flattenNodes(nodes []libraryNode) []Entry {
	entries := make([]Entry, 0)
	for _, node := range nodes {
		flattenNode(&entries, node, nil, "")
	}
	return entries
}

func flattenNode(entries *[]Entry, node libraryNode, path []int, parentPathKey string) string {
	currentPath := append(copyInts(path), node.Slot)
	pathKey := joinPath(currentPath)
	name := fallbackName(node.Label)
	breadcrumbs := append(breadcrumbSegments(path, *entries), name)
	breadcrumb := stringsJoin(breadcrumbs, " / ")

	entry := Entry{
		Path:              currentPath,
		PathKey:           pathKey,
		ParentPathKey:     parentPathKey,
		RecordType:        node.Kind,
		Name:              name,
		Description:       node.Description,
		Breadcrumb:        breadcrumb,
		SearchName:        normalize(name),
		SearchDescription: normalize(node.Description),
		SearchBreadcrumb:  normalize(breadcrumb),
		LabelResolved:     true,
		ChildPathKeys:     []string{},
		IconSprite:        node.IconSprite,
		EntityCount:       0,
		Tags:              map[string]any{},
	}
	entry.SearchText = buildSearchText(entry.Name, entry.Description, entry.Breadcrumb, entry.Tags)

	entryIndex := len(*entries)
	*entries = append(*entries, entry)

	for _, child := range node.Children {
		childPathKey := flattenNode(entries, child, currentPath, pathKey)
		(*entries)[entryIndex].ChildPathKeys = append((*entries)[entryIndex].ChildPathKeys, childPathKey)
	}

	return pathKey
}

func breadcrumbSegments(path []int, entries []Entry) []string {
	if len(path) == 0 {
		return nil
	}

	segments := make([]string, 0)
	current := ""
	for i := 0; i < len(path); i++ {
		if i == 0 {
			current = fmt.Sprintf("%d", path[i])
		} else {
			current = current + "." + fmt.Sprintf("%d", path[i])
		}
		for _, entry := range entries {
			if entry.PathKey == current {
				segments = append(segments, entry.Name)
				break
			}
		}
	}

	return segments
}

func stringsJoin(parts []string, separator string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += separator + parts[i]
	}
	return result
}
