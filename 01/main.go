package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	unknownCommand     = "неизвестная команда"
	nothingInteresting = "ничего интересного"
	corridorName       = "коридор"
	streetName         = "улица"
	backpackName       = "рюкзак"
)

type Item struct {
	name       string
	takeable   bool
	wearable   bool
	showable   bool
	container  bool
	showFormat string
	contents   []*Item
}

func newItem(name string) *Item {
	return &Item{name: name, takeable: true, showable: true}
}

func newFurniture(name, showFormat string) *Item {
	return &Item{name: name, container: true, showable: true, showFormat: showFormat}
}

func newWearable(name string) *Item {
	return &Item{name: name, wearable: true, showable: true}
}

func (i *Item) add(items ...*Item) *Item {
	i.contents = append(i.contents, items...)
	return i
}

func (i *Item) find(name string) (*Item, *Item) {
	for _, item := range i.contents {
		if item.name == name {
			return item, i
		}
		if found, owner := item.find(name); found != nil {
			return found, owner
		}
	}
	return nil, nil
}

func (i *Item) removeChild(target *Item) {
	for idx, item := range i.contents {
		if item == target {
			i.contents = append(i.contents[:idx], i.contents[idx+1:]...)
			return
		}
	}
}

func itemNames(items []*Item) string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.showable {
			names = append(names, item.name)
		}
	}
	return strings.Join(names, ", ")
}

func showItems(items []*Item) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if !item.showable {
			continue
		}
		if !item.container {
			parts = append(parts, item.name)
			continue
		}
		inner := itemNames(item.contents)
		if inner == "" {
			continue
		}
		format := item.showFormat
		if format == "" {
			format = item.name + ": %s"
		}
		parts = append(parts, fmt.Sprintf(format, inner))
	}
	return strings.Join(parts, ", ")
}

type Exit struct {
	name   string
	room   *Room
	locked bool
}

type Room struct {
	name  string
	enter string
	look  func(*Player) string
	apply func(*Player, *Item, string) string
	exits []*Exit
	items []*Item
}

func (r *Room) exitNames() string {
	names := make([]string, 0, len(r.exits))
	for _, exit := range r.exits {
		names = append(names, exit.name)
	}
	return strings.Join(names, ", ")
}

func (r *Room) findExit(name string) *Exit {
	for _, exit := range r.exits {
		if exit.name == name {
			return exit
		}
	}
	return nil
}

func (r *Room) findItem(name string) (*Item, *Item) {
	for _, item := range r.items {
		if item.name == name {
			return item, nil
		}
		if found, owner := item.find(name); found != nil {
			return found, owner
		}
	}
	return nil, nil
}

func (r *Room) removeItem(item, owner *Item) {
	if owner != nil {
		owner.removeChild(item)
		return
	}
	for idx, child := range r.items {
		if child == item {
			r.items = append(r.items[:idx], r.items[idx+1:]...)
			return
		}
	}
}

type Quest struct {
	description string
	done        func(*Player) bool
}

type Player struct {
	room      *Room
	inventory []*Item
	worn      map[string]bool
}

func (p *Player) hasBackpack() bool {
	return p.worn[backpackName]
}

func (p *Player) findInventoryItem(name string) *Item {
	for _, item := range p.inventory {
		if item.name == name {
			return item
		}
	}
	return nil
}

func (p *Player) pendingQuests() string {
	pending := make([]string, 0, len(quests))
	for _, quest := range quests {
		if !quest.done(p) {
			pending = append(pending, quest.description)
		}
	}
	return strings.Join(pending, " и ")
}

var (
	currentPlayer *Player
	quests        []*Quest
)

func describe(room *Room) string {
	var text string
	if room.look != nil {
		text = room.look(currentPlayer)
	}
	ways := room.exitNames()
	switch {
	case text == "":
		return "можно пройти - " + ways
	case ways == "":
		return text
	default:
		return text + ". можно пройти - " + ways
	}
}

func describeEnter(room *Room) string {
	ways := room.exitNames()
	switch {
	case room.enter == "":
		return "можно пройти - " + ways
	case ways == "":
		return room.enter
	default:
		return room.enter + ". можно пройти - " + ways
	}
}

func lookCommand(args []string) string {
	if len(args) != 0 {
		return unknownCommand
	}
	return describe(currentPlayer.room)
}

func goCommand(args []string) string {
	if len(args) != 1 {
		return unknownCommand
	}
	exit := currentPlayer.room.findExit(args[0])
	if exit == nil {
		return "нет пути в " + args[0]
	}
	if exit.locked {
		return "дверь закрыта"
	}
	currentPlayer.room = exit.room
	return describeEnter(currentPlayer.room)
}

func takeCommand(args []string) string {
	if len(args) != 1 {
		return unknownCommand
	}
	item, owner := currentPlayer.room.findItem(args[0])
	if item == nil {
		return "нет такого"
	}
	if !item.takeable {
		return "невозможно"
	}
	if !currentPlayer.hasBackpack() {
		return "некуда класть"
	}
	currentPlayer.room.removeItem(item, owner)
	currentPlayer.inventory = append(currentPlayer.inventory, item)
	return "предмет добавлен в инвентарь: " + item.name
}

func wearCommand(args []string) string {
	if len(args) != 1 {
		return unknownCommand
	}
	item, owner := currentPlayer.room.findItem(args[0])
	if item == nil {
		return "нет такого"
	}
	if !item.wearable {
		return "невозможно"
	}
	currentPlayer.room.removeItem(item, owner)
	currentPlayer.worn[item.name] = true
	return "вы надели: " + item.name
}

func useCommand(args []string) string {
	if len(args) != 2 {
		return unknownCommand
	}
	item := currentPlayer.findInventoryItem(args[0])
	if item == nil {
		return "нет предмета в инвентаре - " + args[0]
	}
	if currentPlayer.room.apply == nil {
		return "не к чему применить"
	}
	answer := currentPlayer.room.apply(currentPlayer, item, args[1])
	if answer == "" {
		return "не к чему применить"
	}
	return answer
}

var handlers = map[string]func([]string) string{
	"осмотреться": lookCommand,
	"идти":        goCommand,
	"взять":       takeCommand,
	"надеть":      wearCommand,
	"применить":   useCommand,
}

func handleCommand(command string) string {
	parts := strings.Split(command, " ")
	if parts[0] == "" {
		return unknownCommand
	}
	handler, ok := handlers[parts[0]]
	if !ok {
		return unknownCommand
	}
	return handler(parts[1:])
}

func initGame() {
	tea := newItem("чай")
	tea.takeable = false

	kitchen := &Room{name: "кухня", enter: "кухня, ничего интересного"}
	corridor := &Room{name: corridorName, enter: nothingInteresting}
	room := &Room{name: "комната", enter: "ты в своей комнате"}
	street := &Room{name: streetName, enter: "на улице весна"}

	kitchen.items = []*Item{
		newFurniture("стол", "на столе: %s").add(tea),
	}
	room.items = []*Item{
		newFurniture("стол", "на столе: %s").add(newItem("ключи"), newItem("конспекты")),
		newFurniture("стул", "на стуле: %s").add(newWearable(backpackName)),
	}

	kitchen.exits = []*Exit{{name: corridorName, room: corridor}}
	corridor.exits = []*Exit{
		{name: "кухня", room: kitchen},
		{name: "комната", room: room},
		{name: streetName, room: street, locked: true},
	}
	room.exits = []*Exit{{name: corridorName, room: corridor}}
	street.exits = []*Exit{{name: "домой", room: corridor}}

	corridor.apply = func(_ *Player, item *Item, target string) string {
		if target != "дверь" || item.name != "ключи" {
			return ""
		}
		if exit := corridor.findExit(streetName); exit != nil {
			exit.locked = false
		}
		return "дверь открыта"
	}

	kitchen.look = func(p *Player) string {
		parts := []string{"ты находишься на кухне"}
		if items := showItems(kitchen.items); items != "" {
			parts = append(parts, items)
		}
		if pending := p.pendingQuests(); pending != "" {
			parts = append(parts, "надо "+pending)
		}
		return strings.Join(parts, ", ")
	}
	corridor.look = func(*Player) string { return nothingInteresting }
	room.look = func(*Player) string {
		if items := showItems(room.items); items != "" {
			return items
		}
		return "пустая комната"
	}
	street.look = func(*Player) string { return nothingInteresting }

	quests = []*Quest{
		{description: "собрать рюкзак", done: func(p *Player) bool { return p.worn[backpackName] }},
		{description: "идти в универ", done: func(*Player) bool { return false }},
	}

	currentPlayer = &Player{room: kitchen, worn: make(map[string]bool)}
}

func main() {
	initGame()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Println(handleCommand(scanner.Text()))
	}
}
